package provider

import "errors"

// Shared sentinel errors for API error classification.
// All provider adapters wrap these via %w so query-level code
// can use errors.Is without importing provider sub-packages.
var (
	ErrRateLimit       = errors.New("rate limit exceeded")
	ErrOverloaded      = errors.New("overloaded")
	ErrServerError     = errors.New("server error")
	ErrAuthentication  = errors.New("authentication failed")
	ErrInvalidRequest  = errors.New("invalid request")
	ErrContextOverflow = errors.New("context window exceeded")
)
