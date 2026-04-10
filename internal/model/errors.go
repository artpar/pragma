package model

import "errors"

// Sentinel errors for the model package.
// Use errors.Is(err, ErrX) for checking; wrap with fmt.Errorf("context: %w", ErrX).

var (
	ErrConversationNotFound  = errors.New("conversation not found")
	ErrMessageNotFound       = errors.New("message not found")
	ErrToolNotFound          = errors.New("tool not found")
	ErrToolAlreadyRegistered = errors.New("tool already registered")
	ErrToolPermissionDenied  = errors.New("tool permission denied")
	ErrProviderUnavailable   = errors.New("provider unavailable")
	ErrStreamClosed          = errors.New("stream closed")
	ErrMaxTokensExceeded     = errors.New("max tokens exceeded")
	ErrInvalidContentType    = errors.New("invalid content type")
	ErrInvalidRole           = errors.New("invalid role")
	ErrContextCancelled      = errors.New("context cancelled")
)
