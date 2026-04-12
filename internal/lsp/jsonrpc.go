package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/artpar/gogent/internal/observe"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Codec implements JSON-RPC 2.0 over Content-Length-framed stdio.
// Responses are matched to requests by id. Notifications (no id) are
// dispatched to registered handlers. Server-to-client requests are
// dispatched to registered request handlers and responded to automatically.
type Codec struct {
	in  *bufio.Reader
	out io.Writer
	wmu sync.Mutex // serializes writes
	seq atomic.Int64

	pending sync.Map // id (int64) -> chan *rpcResponse

	nmu             sync.RWMutex
	notifyHandlers  map[string]func(json.RawMessage)
	requestHandlers map[string]func(json.RawMessage) (any, error)

	closed   atomic.Bool
	closedCh chan struct{}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// RPCError exposes the error code for callers that need to check
// specific JSON-RPC error codes (e.g. -32801 content modified).
type RPCError = rpcError

// rpcMessage is the union used for reading — could be request, response, or notification.
type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // null or number for responses; absent for notifications
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func NewCodec(in io.Reader, out io.Writer) *Codec {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Codec{\n\tin:\t\t\tbufio.NewReaderSize(in, 64*1024),\n\tout:\t\t\tout,\n\tnotifyHandlers...")
	observe.GlobalTrace("return: &Codec{\n\tin:\t\t\tbufio.NewReaderSize(in, 64*1024),\n\tout:\t\t\tout,\n\tnotifyHandlers...")
	observe.GlobalTrace("return: &Codec{\n\tin:\t\t\tbufio.NewReaderSize(in, 64*1024),\n\tout:\t\t\tout,\n\tnotifyHandlers...")
	return &Codec{
		in:              bufio.NewReaderSize(in, 64*1024),
		out:             out,
		notifyHandlers:  make(map[string]func(json.RawMessage)),
		requestHandlers: make(map[string]func(json.RawMessage) (any, error)),
		closedCh:        make(chan struct{}),
	}
}

// OnNotification registers a handler for server-to-client notifications.
func (c *Codec) OnNotification(method string, handler func(json.RawMessage)) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.nmu.Lock()
	defer c.nmu.Unlock()
	c.notifyHandlers[method] = handler
}

// OnRequest registers a handler for server-to-client requests.
// The handler's return value is sent back as the response result.
func (c *Codec) OnRequest(method string, handler func(json.RawMessage) (any, error)) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.nmu.Lock()
	defer c.nmu.Unlock()
	c.requestHandlers[method] = handler
}

// Call sends a JSON-RPC request and waits for the response.
// Blocks until response arrives or ctx is cancelled.
func (c *Codec) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	observe.TraceCtx(ctx, "lsp", "Codec.Call", "enter")
	defer observe.TraceCtx(ctx, "lsp", "Codec.Call", "exit")
	if c.closed.Load() {
		observe.TraceCtx(ctx, "lsp", "Codec.Call", "if: c.closed.Load()")
		observe.TraceCtx(ctx, "lsp", "Codec.Call", "return: nil, ErrConnectionClosed")
		observe.TraceCtx(ctx, "lsp", "Codec.Call", "return: nil, ErrConnectionClosed")
		observe.TraceCtx(ctx, "lsp", "Codec.Call", "return: nil, ErrConnectionClosed")
		return nil, ErrConnectionClosed
	}

	id := c.seq.Add(1)
	ch := make(chan *rpcResponse, 1)
	c.pending.Store(id, ch)
	defer c.pending.Delete(id)

	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	if err := c.writeMessage(req); err != nil {
		observe.TraceCtx(ctx, "lsp", "Codec.Call", "if: err != nil")
		observe.TraceCtx(ctx, "lsp", "Codec.Call", "return: nil, fmt.Errorf(\"write request: %w\", err)")
		observe.TraceCtx(ctx, "lsp", "Codec.Call", "return: nil, fmt.Errorf(\"write request: %w\", err)")
		observe.TraceCtx(ctx, "lsp", "Codec.Call", "return: nil, fmt.Errorf(\"write request: %w\", err)")
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case resp := <-ch:
		observe.TraceCtx(ctx, "lsp", "Codec.Call", "select: resp := <-ch")
		if resp == nil {
			observe.TraceCtx(ctx, "lsp", "Codec.Call", "return: nil, ErrConnectionClosed")
			observe.TraceCtx(ctx, "lsp", "Codec.Call", "return: nil, ErrConnectionClosed")
			observe.TraceCtx(ctx, "lsp", "Codec.Call", "return: nil, ErrConnectionClosed")
			return nil, ErrConnectionClosed
		}
		if resp.Error != nil {
			observe.TraceCtx(ctx, "lsp", "Codec.Call", "return: nil, resp.Error")
			observe.TraceCtx(ctx, "lsp", "Codec.Call", "return: nil, resp.Error")
			observe.TraceCtx(ctx, "lsp", "Codec.Call", "return: nil, resp.Error")
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		observe.TraceCtx(ctx, "lsp", "Codec.Call", "select: <-ctx.Done()")
		return nil, ctx.Err()
	case <-c.closedCh:
		observe.TraceCtx(ctx, "lsp", "Codec.Call", "select: <-c.closedCh")
		return nil, ErrConnectionClosed
	}
}

// Notify sends a JSON-RPC notification (no response expected).
func (c *Codec) Notify(method string, params any) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c.closed.Load() {
		observe.GlobalTrace("if: c.closed.Load()")
		observe.GlobalTrace("return: ErrConnectionClosed")
		observe.GlobalTrace("return: ErrConnectionClosed")
		observe.GlobalTrace("return: ErrConnectionClosed")
		return ErrConnectionClosed
	}
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	observe.GlobalTrace("return: c.writeMessage(req)")
	observe.GlobalTrace("return: c.writeMessage(req)")
	observe.GlobalTrace("return: c.writeMessage(req)")
	return c.writeMessage(req)
}

// ReadLoop reads messages from the input stream and dispatches them.
// Call this in a goroutine. Returns when the stream closes or an error occurs.
func (c *Codec) ReadLoop() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	defer c.close()
	for {
		observe.GlobalTrace("for: true")
		msg, err := c.readMessage()
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			return
		}
		c.dispatch(msg)
	}
}

// Close signals that the codec is shutting down.
func (c *Codec) Close() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.close()
}

func (c *Codec) close() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if c.closed.CompareAndSwap(false, true) {
		observe.GlobalTrace("if: c.closed.CompareAndSwap(false, true)")
		close(c.closedCh)

		c.pending.Range(func(key, value any) bool {
			ch := value.(chan *rpcResponse)
			select {
			case ch <- nil:
			default:
			}
			return true
		})
	}
}

func (c *Codec) dispatch(msg *rpcMessage) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if msg.Method == "" && msg.ID != nil {
		observe.GlobalTrace("if: msg.Method == \"\" && msg.ID != nil")
		c.dispatchResponse(msg)
		return
	}

	if msg.Method != "" && msg.ID != nil {
		observe.GlobalTrace("if: msg.Method != \"\" && msg.ID != nil")
		c.dispatchRequest(msg)
		return
	}

	if msg.Method != "" {
		observe.GlobalTrace("if: msg.Method != \"\"")
		c.dispatchNotification(msg)
		return
	}
}

func (c *Codec) dispatchResponse(msg *rpcMessage) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var id int64
	if err := json.Unmarshal(msg.ID, &id); err != nil {
		observe.GlobalTrace("if: err != nil")
		// Try float64 (some servers send float ids)
		var fid float64
		if err := json.Unmarshal(msg.ID, &fid); err != nil {
			observe.GlobalTrace("if: err != nil")
			return
		}
		id = int64(fid)
	}

	val, ok := c.pending.Load(id)
	if !ok {
		observe.GlobalTrace("if: !ok")
		return
	}
	ch := val.(chan *rpcResponse)
	resp := &rpcResponse{
		JSONRPC: msg.JSONRPC,
		Result:  msg.Result,
		Error:   msg.Error,
	}
	select {
	case ch <- resp:
		observe.GlobalTrace("select: ch <- resp")
	default:
		observe.GlobalTrace("select: default")
	}
}

func (c *Codec) dispatchRequest(msg *rpcMessage) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.nmu.RLock()
	handler, ok := c.requestHandlers[msg.Method]
	c.nmu.RUnlock()

	if !ok {
		observe.GlobalTrace("if: !ok")

		c.sendResponse(msg.ID, nil, &rpcError{Code: -32601, Message: "method not found"})
		return
	}

	result, err := handler(msg.Params)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		c.sendResponse(msg.ID, nil, &rpcError{Code: -32603, Message: err.Error()})
		return
	}
	c.sendResponse(msg.ID, result, nil)
}

func (c *Codec) sendResponse(id json.RawMessage, result any, rpcErr *rpcError) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	resp := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result,omitempty"`
		Error   *rpcError       `json:"error,omitempty"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   rpcErr,
	}

	_ = c.writeMessage(resp)
}

func (c *Codec) dispatchNotification(msg *rpcMessage) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.nmu.RLock()
	handler, ok := c.notifyHandlers[msg.Method]
	c.nmu.RUnlock()
	if ok {
		observe.GlobalTrace("if: ok")
		handler(msg.Params)
	}
}

func (c *Codec) writeMessage(msg any) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	body, err := json.Marshal(msg)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"marshal message: %w\", err)")
		observe.GlobalTrace("return: fmt.Errorf(\"marshal message: %w\", err)")
		observe.GlobalTrace("return: fmt.Errorf(\"marshal message: %w\", err)")
		return fmt.Errorf("marshal message: %w", err)
	}

	c.wmu.Lock()
	defer c.wmu.Unlock()

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(c.out, header); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"write header: %w\", err)")
		observe.GlobalTrace("return: fmt.Errorf(\"write header: %w\", err)")
		observe.GlobalTrace("return: fmt.Errorf(\"write header: %w\", err)")
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := c.out.Write(body); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"write body: %w\", err)")
		observe.GlobalTrace("return: fmt.Errorf(\"write body: %w\", err)")
		observe.GlobalTrace("return: fmt.Errorf(\"write body: %w\", err)")
		return fmt.Errorf("write body: %w", err)
	}
	observe.GlobalTrace("return: nil")
	observe.GlobalTrace("return: nil")
	observe.GlobalTrace("return: nil")
	return nil
}

func (c *Codec) readMessage() (*rpcMessage, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	contentLength := -1

	for {
		observe.GlobalTrace("for: true")
		line, err := c.in.ReadString('\n')
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, err")
			observe.GlobalTrace("return: nil, err")
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			observe.GlobalTrace("if: line == \"\"")
			break
		}

		if strings.HasPrefix(line, "Content-Length:") {
			observe.GlobalTrace("if: strings.HasPrefix(line, \"Content-Length:\")")
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			n, err := strconv.Atoi(val)
			if err != nil {
				observe.GlobalTrace("if: err != nil")
				observe.GlobalTrace("return: nil, fmt.Errorf(\"invalid Content-Length %q: %w\", val, err)")
				observe.GlobalTrace("return: nil, fmt.Errorf(\"invalid Content-Length %q: %w\", val, err)")
				observe.GlobalTrace("return: nil, fmt.Errorf(\"invalid Content-Length %q: %w\", val, err)")
				return nil, fmt.Errorf("invalid Content-Length %q: %w", val, err)
			}
			contentLength = n
		}

	}

	if contentLength < 0 {
		observe.GlobalTrace("if: contentLength < 0")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"missing Content-Length header\")")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"missing Content-Length header\")")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"missing Content-Length header\")")
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(c.in, body); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"read body: %w\", err)")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"read body: %w\", err)")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"read body: %w\", err)")
		return nil, fmt.Errorf("read body: %w", err)
	}

	var msg rpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"unmarshal message: %w\", err)")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"unmarshal message: %w\", err)")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"unmarshal message: %w\", err)")
		return nil, fmt.Errorf("unmarshal message: %w", err)
	}
	observe.GlobalTrace("return: &msg, nil")
	observe.GlobalTrace("return: &msg, nil")
	observe.GlobalTrace("return: &msg, nil")
	return &msg, nil
}
