package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"

	"calabrass-tui/internal/termsafe"
)

// ConnKind says how a connection failed.
type ConnKind int

const (
	// ConnRefused: nothing answered. The host is not running, or not there.
	ConnRefused ConnKind = iota
	// ConnLost: it was answering, then the connection broke or the host went away.
	ConnLost
	// ConnIdle: the connection is open but the host stopped sending anything.
	ConnIdle
	// ConnTimeout: the host did not answer in time.
	ConnTimeout
)

// ConnError is a failure to talk to the host: it is down, restarting, unreachable
// or hung. It is not the host saying no (that is an APIError).
//
// Its message is written for a person. The Go networking error underneath names
// the URL, the port and an operating-system errno, which is noise on a screen and
// not something to show; it is kept for Unwrap, and for logs, not for display.
type ConnError struct {
	Kind ConnKind
	Err  error
}

func (e *ConnError) Error() string {
	switch e.Kind {
	case ConnLost:
		return "lost the connection to the Calabrass host"
	case ConnIdle:
		return "the Calabrass host stopped responding"
	case ConnTimeout:
		return "the Calabrass host took too long to answer"
	default:
		return "can't reach the Calabrass host"
	}
}

func (e *ConnError) Unwrap() error { return e.Err }

// IsConnError reports whether err is a failure to reach, or keep, the host.
func IsConnError(err error) bool {
	var ce *ConnError
	return errors.As(err, &ce)
}

// BadReplyError means something answered, but not as a Calabrass host does: an
// HTML error page from a proxy, another program on the port, a damaged body.
type BadReplyError struct {
	Status int
	Detail string
}

func (e *BadReplyError) Error() string {
	if e.Status != 0 {
		return "the host sent a reply this program cannot read (HTTP " + itoa(e.Status) + "). Is something else using that port?"
	}
	return "the host sent a reply this program cannot read. Is something else using that port?"
}

// IsBadReply reports whether err is a BadReplyError.
func IsBadReply(err error) bool {
	var be *BadReplyError
	return errors.As(err, &be)
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// Explain words any error from this package for the screen: short, one line, no
// URLs, no operating-system text, and nothing from the network that could drive
// the terminal.
func Explain(err error) string {
	if err == nil {
		return ""
	}
	var ce *ConnError
	var be *BadReplyError
	var ae *APIError
	switch {
	case errors.As(err, &ce):
		return ce.Error()
	case errors.As(err, &be):
		return be.Error()
	case errors.As(err, &ae):
		return termsafe.Clip(ae.Error(), 300)
	case errors.Is(err, context.DeadlineExceeded):
		return "the Calabrass host took too long to answer"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	}
	return termsafe.Clip(err.Error(), 200)
}

// wrapTransport turns an error from the HTTP client into a ConnError. An error
// that is not about the connection (a bad request body, say) is returned as it
// came. When the caller cancelled, that is returned as the cancellation, so the
// UI can tell "I stopped it" from "it broke".
func wrapTransport(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil && errors.Is(ctx.Err(), context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.Canceled) {
		return err
	}

	var ce *ConnError
	if errors.As(err, &ce) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &ConnError{Kind: ConnTimeout, Err: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &ConnError{Kind: ConnTimeout, Err: err}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNABORTED) || errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrClosed) {
		return &ConnError{Kind: ConnLost, Err: err}
	}
	var op *net.OpError
	if errors.As(err, &op) {
		if op.Op == "dial" {
			return &ConnError{Kind: ConnRefused, Err: err}
		}
		return &ConnError{Kind: ConnLost, Err: err}
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		// A URL error that is none of the above still means the request did not
		// complete. Treat it as a connection that did not come up.
		if strings.Contains(strings.ToLower(ue.Error()), "connection") ||
			strings.Contains(strings.ToLower(ue.Error()), "refused") ||
			strings.Contains(strings.ToLower(ue.Error()), "no such host") ||
			strings.Contains(strings.ToLower(ue.Error()), "network") {
			return &ConnError{Kind: ConnRefused, Err: err}
		}
		return &ConnError{Kind: ConnLost, Err: err}
	}
	return err
}
