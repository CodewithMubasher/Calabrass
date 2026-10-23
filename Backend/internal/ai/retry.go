package ai

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/zendev-sh/goai"
)

// Retryable reports whether a provider failure is the kind a compatible
// fallback might survive: a rate limit, a temporary network problem, an
// overloaded endpoint, or a server-side 5xx. It deliberately says no to
// cancellation, context overflow, authentication failures, and malformed
// requests, because another model will fail the same way and the state would be
// lost for nothing.
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var apiErr *goai.APIError
	if errors.As(err, &apiErr) {
		if apiErr.IsRetryable {
			return true
		}
		switch apiErr.StatusCode {
		case 408, 409, 425, 429:
			return true
		}
		return apiErr.StatusCode >= 500 && apiErr.StatusCode <= 599
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	text := strings.ToLower(err.Error())
	for _, needle := range []string{
		"rate limit",
		"too many requests",
		"429",
		"overloaded",
		"temporarily unavailable",
		"service unavailable",
		"timeout",
		"timed out",
		"connection reset",
		"connection refused",
		"broken pipe",
		"unexpected eof",
		"bad gateway",
		"gateway timeout",
		"502",
		"503",
		"504",
	} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
