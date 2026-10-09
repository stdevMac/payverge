package llm

import (
	"errors"
	"fmt"
)

// Sentinel error classes callers can match with errors.Is to preserve their
// existing user-facing fallback copy.
var (
	ErrAuth              = errors.New("llm: authentication failed")
	ErrRateLimited       = errors.New("llm: rate limited")
	ErrUpstream          = errors.New("llm: upstream provider error")
	ErrMalformedResponse = errors.New("llm: malformed provider response")
	ErrStructuredOutput  = errors.New("llm: structured output invalid")
)

// APIError carries the provider's HTTP status and message for explicit,
// access-controlled diagnostics while wrapping one of the sentinels above.
// Error deliberately omits Message because provider errors can echo request
// content; ordinary wrapped-error logging must never expose that payload.
type APIError struct {
	Class   error // one of the sentinels
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%v (status %d)", e.Class, e.Status)
}

func (e *APIError) Unwrap() error { return e.Class }
