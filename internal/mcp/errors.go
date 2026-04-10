package mcp

import "errors"

var (
	ErrServerNotConnected = errors.New("mcp server not connected")
	ErrToolCallFailed     = errors.New("mcp tool call failed")
	ErrToolCallTimeout    = errors.New("mcp tool call timed out")
	ErrInvalidConfig      = errors.New("invalid mcp server config")
)
