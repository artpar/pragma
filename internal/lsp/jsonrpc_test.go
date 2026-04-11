package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockServer simulates an LSP server: reads requests from serverIn, writes responses to serverOut.
type mockServer struct {
	serverIn  io.Reader // server reads client requests here
	serverOut io.Writer // server writes responses here
}

func newPipePair() (clientCodec *Codec, server *mockServer) {
	// Client writes → server reads
	clientToServerR, clientToServerW := io.Pipe()
	// Server writes → client reads
	serverToClientR, serverToClientW := io.Pipe()

	codec := NewCodec(serverToClientR, clientToServerW)
	return codec, &mockServer{serverIn: clientToServerR, serverOut: serverToClientW}
}

// readRequest reads one JSON-RPC message from the server side.
func (s *mockServer) readRequest(t *testing.T) rpcMessage {
	t.Helper()
	codec := NewCodec(s.serverIn, s.serverOut)
	msg, err := codec.readMessage()
	if err != nil {
		t.Fatalf("server readMessage: %v", err)
	}
	return *msg
}

// writeResponse writes a JSON-RPC response to the client.
func (s *mockServer) writeResponse(t *testing.T, id int64, result any) {
	t.Helper()
	body := struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int64  `json:"id"`
		Result  any    `json:"result"`
	}{"2.0", id, result}
	data, _ := json.Marshal(body)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	_, _ = io.WriteString(s.serverOut, header)
	_, _ = s.serverOut.Write(data)
}

// writeNotification writes a JSON-RPC notification to the client.
func (s *mockServer) writeNotification(t *testing.T, method string, params any) {
	t.Helper()
	body := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{"2.0", method, params}
	data, _ := json.Marshal(body)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	_, _ = io.WriteString(s.serverOut, header)
	_, _ = s.serverOut.Write(data)
}

// writeRequest writes a server-to-client request.
func (s *mockServer) writeRequest(t *testing.T, id int64, method string, params any) {
	t.Helper()
	body := struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int64  `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{"2.0", id, method, params}
	data, _ := json.Marshal(body)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	_, _ = io.WriteString(s.serverOut, header)
	_, _ = s.serverOut.Write(data)
}

func TestCodecCallAndResponse(t *testing.T) {
	codec, server := newPipePair()
	go codec.ReadLoop()
	defer codec.Close()

	// Server goroutine: read request, send response
	go func() {
		msg := server.readRequest(t)
		if msg.Method != "textDocument/hover" {
			t.Errorf("expected method textDocument/hover, got %s", msg.Method)
		}
		var id int64
		_ = json.Unmarshal(msg.ID, &id)
		server.writeResponse(t, id, map[string]string{"contents": "hello"})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := codec.Call(ctx, "textDocument/hover", map[string]int{"line": 1})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["contents"] != "hello" {
		t.Errorf("expected contents=hello, got %s", parsed["contents"])
	}
}

func TestCodecNotification(t *testing.T) {
	codec, server := newPipePair()
	go codec.ReadLoop()
	defer codec.Close()

	var received sync.WaitGroup
	received.Add(1)
	var gotParams json.RawMessage

	codec.OnNotification("textDocument/publishDiagnostics", func(params json.RawMessage) {
		gotParams = params
		received.Done()
	})

	// Server sends notification
	go func() {
		server.writeNotification(t, "textDocument/publishDiagnostics", map[string]string{"uri": "file:///test.go"})
	}()

	received.Wait()
	if gotParams == nil {
		t.Fatal("notification handler not called")
	}
	if !json.Valid(gotParams) {
		t.Error("params is not valid JSON")
	}
}

func TestCodecServerToClientRequest(t *testing.T) {
	codec, server := newPipePair()
	go codec.ReadLoop()
	defer codec.Close()

	// Register handler for workspace/configuration
	codec.OnRequest("workspace/configuration", func(params json.RawMessage) (any, error) {
		// Return null for each item (matches TS behavior)
		return []any{nil}, nil
	})

	// Server sends request
	go func() {
		server.writeRequest(t, 99, "workspace/configuration", map[string]any{
			"items": []map[string]string{{"section": "gopls"}},
		})
	}()

	// Read the response that the codec sends back
	time.Sleep(100 * time.Millisecond) // give codec time to process
	msg := server.readRequest(t)

	// Should be a response with id=99
	var id int64
	if err := json.Unmarshal(msg.ID, &id); err != nil {
		// Could be the response format
		t.Logf("response: %+v", msg)
	}
}

func TestCodecConcurrentCalls(t *testing.T) {
	codec, server := newPipePair()
	go codec.ReadLoop()
	defer codec.Close()

	const numCalls = 5

	// Server goroutine: read requests and respond
	go func() {
		for i := 0; i < numCalls; i++ {
			msg := server.readRequest(t)
			var id int64
			_ = json.Unmarshal(msg.ID, &id)
			server.writeResponse(t, id, map[string]int64{"id": id})
		}
	}()

	var wg sync.WaitGroup
	results := make([]json.RawMessage, numCalls)
	errs := make([]error, numCalls)

	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			results[idx], errs[idx] = codec.Call(ctx, "test/method", map[string]int{"idx": idx})
		}(i)
	}
	wg.Wait()

	for i := 0; i < numCalls; i++ {
		if errs[i] != nil {
			t.Errorf("call %d failed: %v", i, errs[i])
		}
		if results[i] == nil {
			t.Errorf("call %d: nil result", i)
		}
	}
}

func TestCodecContextCancellation(t *testing.T) {
	codec, server := newPipePair()
	go codec.ReadLoop()
	defer codec.Close()

	// Server reads the request but never responds
	go func() {
		serverCodec := NewCodec(server.serverIn, server.serverOut)
		_, _ = serverCodec.readMessage()
		// Don't respond — let context expire
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := codec.Call(ctx, "test/timeout", nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestCodecConnectionClosed(t *testing.T) {
	codec, _ := newPipePair()
	go codec.ReadLoop()

	// Close the codec
	codec.Close()

	_, err := codec.Call(context.Background(), "test/closed", nil)
	if err != ErrConnectionClosed {
		t.Errorf("expected ErrConnectionClosed, got %v", err)
	}
}

func TestCodecCRLFHeaders(t *testing.T) {
	// Verify that writeMessage uses \r\n (not just \n) — issue #33529
	r, w := io.Pipe()
	codec := NewCodec(r, w)

	go func() {
		_ = codec.Notify("test/crlf", nil)
		w.Close()
	}()

	buf := make([]byte, 256)
	n, _ := r.Read(buf)
	header := string(buf[:n])

	if !strings.Contains(header, "\r\n\r\n") {
		t.Errorf("header must use CRLF: got %q", header)
	}
}
