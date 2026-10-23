package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/zendev-sh/goai"
)

func TestRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"rate limit status", &goai.APIError{StatusCode: 429, Message: "rate limit"}, true},
		{"server error", &goai.APIError{StatusCode: 503, Message: "unavailable"}, true},
		{"auth failure", &goai.APIError{StatusCode: 401, Message: "bad key"}, false},
		{"bad request", &goai.APIError{StatusCode: 400, Message: "invalid model"}, false},
		{"cancelled", context.Canceled, false},
		{"network text", errors.New("connection reset by peer"), true},
		{"unknown", errors.New("the model produced an invalid tool call"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Retryable(tc.err); got != tc.want {
				t.Fatalf("Retryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
