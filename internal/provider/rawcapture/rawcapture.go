package rawcapture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const EnvDir = "PRAGMA_RAW_HTTP_CAPTURE_DIR"

type contextKey string

const (
	traceIDKey contextKey = "trace_id"
	spanIDKey  contextKey = "span_id"
)

type Transport struct {
	base http.RoundTripper
	dir  string
	seq  uint64
}

type requestMeta struct {
	Sequence      uint64 `json:"sequence"`
	TraceID       string `json:"trace_id,omitempty"`
	SpanID        string `json:"span_id,omitempty"`
	Method        string `json:"method"`
	URL           string `json:"url"`
	Host          string `json:"host"`
	StartedAt     string `json:"started_at"`
	RequestBytes  int    `json:"request_bytes"`
	RequestSHA256 string `json:"request_sha256,omitempty"`
	Stream        *bool  `json:"stream,omitempty"`
}

type responseMeta struct {
	Sequence       uint64 `json:"sequence"`
	TraceID        string `json:"trace_id,omitempty"`
	SpanID         string `json:"span_id,omitempty"`
	Status         string `json:"status,omitempty"`
	StatusCode     int    `json:"status_code,omitempty"`
	StartedAt      string `json:"started_at"`
	CompletedAt    string `json:"completed_at,omitempty"`
	ResponseBytes  int64  `json:"response_bytes,omitempty"`
	ResponseSHA256 string `json:"response_sha256,omitempty"`
	Error          string `json:"error,omitempty"`
}

func WithTrace(ctx context.Context, traceID, spanID string) context.Context {
	ctx = context.WithValue(ctx, traceIDKey, traceID)
	ctx = context.WithValue(ctx, spanIDKey, spanID)
	return ctx
}

func NewTransport(dir string, base http.RoundTripper) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &Transport{base: base, dir: dir}
}

func EnabledDirFromEnv() string {
	return strings.TrimSpace(os.Getenv(EnvDir))
}

func HTTPClientFromEnv(timeout time.Duration) (*http.Client, bool) {
	dir := EnabledDirFromEnv()
	if dir == "" {
		return nil, false
	}
	base := http.DefaultTransport
	if tr, ok := http.DefaultTransport.(*http.Transport); ok {
		base = tr.Clone()
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: NewTransport(dir, base),
	}, true
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.dir == "" {
		return t.base.RoundTrip(req)
	}

	seq := atomic.AddUint64(&t.seq, 1)
	traceID, _ := req.Context().Value(traceIDKey).(string)
	spanID, _ := req.Context().Value(spanIDKey).(string)
	captureDir, ok := t.prepareCaptureDir(seq, traceID, spanID)
	if !ok {
		return t.base.RoundTrip(req)
	}

	body, err := drainRequestBody(req)
	if err == nil {
		writeFile(filepath.Join(captureDir, "request.json"), body, 0o600)
	}
	writeJSON(filepath.Join(captureDir, "request.headers.json"), redactHeaders(req.Header), 0o600)
	writeJSON(filepath.Join(captureDir, "request.meta.json"), requestMeta{
		Sequence:      seq,
		TraceID:       traceID,
		SpanID:        spanID,
		Method:        req.Method,
		URL:           req.URL.String(),
		Host:          req.Host,
		StartedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		RequestBytes:  len(body),
		RequestSHA256: sha256Hex(body),
		Stream:        detectStream(body),
	}, 0o600)

	resp, roundTripErr := t.base.RoundTrip(req)
	if roundTripErr != nil {
		writeJSON(filepath.Join(captureDir, "response.meta.json"), responseMeta{
			Sequence:    seq,
			TraceID:     traceID,
			SpanID:      spanID,
			StartedAt:   time.Now().UTC().Format(time.RFC3339Nano),
			CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Error:       roundTripErr.Error(),
		}, 0o600)
		return nil, roundTripErr
	}
	if resp == nil || resp.Body == nil {
		return resp, nil
	}

	writeJSON(filepath.Join(captureDir, "response.headers.json"), redactHeaders(resp.Header), 0o600)
	rawPath := filepath.Join(captureDir, "response.raw")
	rawFile, err := os.OpenFile(rawPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return resp, nil
	}

	meta := responseMeta{
		Sequence:   seq,
		TraceID:    traceID,
		SpanID:     spanID,
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
		StartedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	writeJSON(filepath.Join(captureDir, "response.meta.json"), meta, 0o600)
	resp.Body = &recordingBody{
		rc:       resp.Body,
		file:     rawFile,
		metaPath: filepath.Join(captureDir, "response.meta.json"),
		meta:     meta,
		hash:     sha256.New(),
	}
	return resp, nil
}

func (t *Transport) prepareCaptureDir(seq uint64, traceID, spanID string) (string, bool) {
	name := fmt.Sprintf("%06d", seq)
	if traceID != "" {
		shortTrace := traceID
		if len(shortTrace) > 12 {
			shortTrace = shortTrace[:12]
		}
		name += "-" + sanitize(shortTrace)
	}
	if spanID != "" {
		shortSpan := spanID
		if len(shortSpan) > 12 {
			shortSpan = shortSpan[:12]
		}
		name += "-" + sanitize(shortSpan)
	}
	captureDir := filepath.Join(t.dir, name)
	if err := os.MkdirAll(captureDir, 0o700); err != nil {
		return "", false
	}
	return captureDir, true
}

type recordingBody struct {
	rc       io.ReadCloser
	file     *os.File
	metaPath string
	meta     responseMeta
	hash     hashWriter
	bytes    int64
	once     sync.Once
}

type hashWriter interface {
	io.Writer
	Sum([]byte) []byte
}

func (b *recordingBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 {
		chunk := p[:n]
		_, _ = b.file.Write(chunk)
		_, _ = b.hash.Write(chunk)
		b.bytes += int64(n)
	}
	return n, err
}

func (b *recordingBody) Close() error {
	var closeErr error
	b.once.Do(func() {
		closeErr = b.rc.Close()
		_ = b.file.Close()
		b.meta.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
		b.meta.ResponseBytes = b.bytes
		b.meta.ResponseSHA256 = hex.EncodeToString(b.hash.Sum(nil))
		writeJSON(b.metaPath, b.meta, 0o600)
	})
	return closeErr
}

func drainRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	return body, nil
}

func redactHeaders(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, values := range h {
		cp := append([]string(nil), values...)
		if isSecretHeader(k) {
			for i := range cp {
				cp[i] = "<redacted>"
			}
		}
		out[k] = cp
	}
	return out
}

func isSecretHeader(k string) bool {
	switch strings.ToLower(k) {
	case "authorization", "x-api-key", "api-key", "proxy-authorization":
		return true
	default:
		return false
	}
}

func detectStream(body []byte) *bool {
	var v struct {
		Stream *bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &v); err != nil || v.Stream == nil {
		return nil
	}
	return v.Stream
}

func writeJSON(path string, value any, perm os.FileMode) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return
	}
	data = append(data, '\n')
	writeFile(path, data, perm)
}

func writeFile(path string, data []byte, perm os.FileMode) {
	_ = os.WriteFile(path, data, perm)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
