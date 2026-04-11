package lsp

import "errors"

var (
	ErrNotInitialized  = errors.New("lsp: server not initialized")
	ErrServerCrashed   = errors.New("lsp: server crashed")
	ErrMaxRestarts     = errors.New("lsp: max restarts exceeded")
	ErrNoServerForFile = errors.New("lsp: no server configured for file type")
	ErrInvalidConfig   = errors.New("lsp: invalid config")
	ErrRequestFailed   = errors.New("lsp: request failed")
	ErrServerStopping  = errors.New("lsp: server is stopping")
	ErrConnectionClosed = errors.New("lsp: connection closed")
)
