package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type rawHTTPRequestMeta struct {
	Method string `json:"method"`
	URL    string `json:"url"`
}

func replayRawHTTPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "raw-http <capture-dir>",
		Short: "Replay a captured raw HTTP LLM request",
		Args:  cobra.ExactArgs(1),
		RunE:  replayRawHTTPRun,
	}
	cmd.Flags().String("base-url", "", "override scheme/host/base path while preserving captured endpoint")
	cmd.Flags().String("api-key-env", "LILAC_API_KEY", "environment variable containing the replay API key")
	return cmd
}

func replayRawHTTPRun(cmd *cobra.Command, args []string) error {
	dir := args[0]
	body, err := os.ReadFile(filepath.Join(dir, "request.json"))
	if err != nil {
		return fmt.Errorf("read request.json: %w", err)
	}
	var meta rawHTTPRequestMeta
	if err := readJSONFile(filepath.Join(dir, "request.meta.json"), &meta); err != nil {
		return fmt.Errorf("read request.meta.json: %w", err)
	}
	if meta.Method == "" {
		meta.Method = http.MethodPost
	}
	targetURL := meta.URL
	if override, _ := cmd.Flags().GetString("base-url"); override != "" {
		targetURL, err = overrideCapturedURL(meta.URL, override)
		if err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(cmd.Context(), meta.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	var headers map[string][]string
	_ = readJSONFile(filepath.Join(dir, "request.headers.json"), &headers)
	for k, values := range headers {
		if skipReplayHeader(k) {
			continue
		}
		for _, v := range values {
			if v != "<redacted>" {
				req.Header.Add(k, v)
			}
		}
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	apiKeyEnv, _ := cmd.Flags().GetString("api-key-env")
	if apiKey := strings.TrimSpace(os.Getenv(apiKeyEnv)); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	fmt.Fprintf(os.Stderr, "HTTP %s\n", resp.Status)
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_, _ = os.Stdout.Write(respBody)
	if len(respBody) > 0 && respBody[len(respBody)-1] != '\n' {
		fmt.Fprintln(os.Stdout)
	}
	printRawHTTPSummary(respBody)
	return nil
}

func overrideCapturedURL(captured, base string) (string, error) {
	capturedURL, err := url.Parse(captured)
	if err != nil {
		return "", fmt.Errorf("parse captured URL: %w", err)
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return "", fmt.Errorf("base-url must include scheme and host")
	}
	result := *baseURL
	basePath := strings.TrimRight(baseURL.Path, "/")
	capturedPath := capturedURL.EscapedPath()
	if basePath != "" && strings.HasPrefix(capturedPath, basePath+"/") {
		result.Path = capturedPath
	} else {
		result.Path = basePath + "/" + strings.TrimLeft(capturedPath, "/")
	}
	result.RawQuery = capturedURL.RawQuery
	return result.String(), nil
}

func skipReplayHeader(k string) bool {
	switch strings.ToLower(k) {
	case "authorization", "content-length", "host":
		return true
	default:
		return false
	}
}

func printRawHTTPSummary(body []byte) {
	summary := summarizeSSE(body)
	if summary == "" {
		summary = summarizeJSONResponse(body)
	}
	if summary != "" {
		fmt.Fprintln(os.Stderr, summary)
	}
}

func summarizeSSE(body []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	var text strings.Builder
	var finish string
	var toolCalls int
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(data), &obj); err != nil {
			continue
		}
		for _, choice := range asSlice(obj["choices"]) {
			ch, _ := choice.(map[string]any)
			if f, _ := ch["finish_reason"].(string); f != "" {
				finish = f
			}
			delta, _ := ch["delta"].(map[string]any)
			if c, _ := delta["content"].(string); c != "" {
				text.WriteString(c)
			}
			if calls := asSlice(delta["tool_calls"]); len(calls) > 0 {
				toolCalls += len(calls)
			}
		}
	}
	if finish == "" && toolCalls == 0 && text.Len() == 0 {
		return ""
	}
	return fmt.Sprintf("summary: finish_reason=%q tool_call_deltas=%d text_prefix=%q", finish, toolCalls, trimSummary(text.String()))
}

func summarizeJSONResponse(body []byte) string {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}
	var finish string
	var toolCalls int
	var text string
	for _, choice := range asSlice(obj["choices"]) {
		ch, _ := choice.(map[string]any)
		if f, _ := ch["finish_reason"].(string); f != "" {
			finish = f
		}
		msg, _ := ch["message"].(map[string]any)
		if c, _ := msg["content"].(string); c != "" {
			text += c
		}
		toolCalls += len(asSlice(msg["tool_calls"]))
	}
	return fmt.Sprintf("summary: finish_reason=%q tool_calls=%d text_prefix=%q", finish, toolCalls, trimSummary(text))
}

func asSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func trimSummary(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 160 {
		return s[:160] + "..."
	}
	return s
}

func readJSONFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
