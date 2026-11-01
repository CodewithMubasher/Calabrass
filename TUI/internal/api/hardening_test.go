package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sseServer(t *testing.T, write func(w http.ResponseWriter, flush func())) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		write(w, func() { w.(http.Flusher).Flush() })
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL)
}

func send(c *Client, ctx context.Context) ([]Event, error) {
	var got []Event
	err := c.Send(ctx, "c1", SendRequest{Content: "hi"}, func(e Event) { got = append(got, e) })
	return got, err
}

func TestAHostThatIsDownIsAConnErrorWithNothingTechnicalInIt(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close() // nothing listens there now

	c := New(addr)
	for name, call := range map[string]func() error{
		"health": func() error { return c.Health(context.Background()) },
		"chats":  func() error { _, err := c.Chats(context.Background()); return err },
		"send":   func() error { _, err := send(c, context.Background()); return err },
	} {
		err := call()
		if !IsConnError(err) {
			t.Fatalf("%s: %T %v is not a ConnError", name, err, err)
		}
		msg := Explain(err)
		for _, leak := range []string{"127.0.0.1", "dial", "tcp", "connect:", "http://", "refused", ":"} {
			if strings.Contains(msg, leak) {
				t.Errorf("%s: %q leaks %q", name, msg, leak)
			}
		}
		if msg != "can't reach the Calabrass host" {
			t.Errorf("%s: message = %q", name, msg)
		}
	}
}

func TestACancelledCallIsNotAConnectionFailure(t *testing.T) {
	c := sseServer(t, func(w http.ResponseWriter, flush func()) {
		flush()
		time.Sleep(2 * time.Second)
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	_, err := send(c, ctx)
	if !errors.Is(err, context.Canceled) || IsConnError(err) {
		t.Fatalf("Esc must read as a cancel, got %T %v", err, err)
	}
}

func TestAReplyThatEndsWithoutDoneIsALostConnectionNotAnAnswer(t *testing.T) {
	c := sseServer(t, func(w http.ResponseWriter, flush func()) {
		fmt.Fprint(w, "event: token\ndata: {\"text\":\"half an ans\"}\n\n")
		flush() // ...and the host dies: the connection closes cleanly with no "done"
	})
	events, err := send(c, context.Background())
	var ce *ConnError
	if !errors.As(err, &ce) || ce.Kind != ConnLost {
		t.Fatalf("a truncated reply must be ConnLost, got %T %v", err, err)
	}
	if len(events) != 1 || events[0].Text != "half an ans" {
		t.Fatalf("what did arrive must still be delivered: %+v", events)
	}
}

func TestAFinishedReplyIsNotAnError(t *testing.T) {
	for name, last := range map[string]string{
		"done":  "event: done\ndata: {\"message\":{\"id\":\"m\"}}\n\n",
		"error": "event: error\ndata: {\"message\":\"the provider said no\"}\n\n",
	} {
		c := sseServer(t, func(w http.ResponseWriter, flush func()) {
			fmt.Fprint(w, "event: token\ndata: {\"text\":\"x\"}\n\n"+last)
		})
		if _, err := send(c, context.Background()); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestAConnectionThatDiesMidReplyIsLost(t *testing.T) {
	c := sseServer(t, func(w http.ResponseWriter, flush func()) {
		fmt.Fprint(w, "event: token\ndata: {\"text\":\"a\"}\n\n")
		flush()
		// Break the TCP connection without finishing the chunked response.
		hj, _, _ := w.(http.Hijacker).Hijack()
		_ = hj.Close()
	})
	_, err := send(c, context.Background())
	if !IsConnError(err) {
		t.Fatalf("a broken connection must be a ConnError, got %T %v", err, err)
	}
}

func TestAStalledHostThatPromisedHeartbeatsIsCutOff(t *testing.T) {
	oldFloor, oldCheck := idleFloor, idleCheck
	idleFloor, idleCheck = 100*time.Millisecond, 20*time.Millisecond
	defer func() { idleFloor, idleCheck = oldFloor, oldCheck }()
	oldBeats := idleBeatsMax
	idleBeatsMax = 0
	defer func() { idleBeatsMax = oldBeats }()

	release := make(chan struct{})
	defer close(release)
	c := sseServer(t, func(w http.ResponseWriter, flush func()) {
		fmt.Fprint(w, ": calabrass-heartbeat 1\n\n")
		flush()
		<-release // the host hangs: connection open, nothing more is sent
	})

	started := time.Now()
	_, err := send(c, context.Background())
	var ce *ConnError
	if !errors.As(err, &ce) || ce.Kind != ConnIdle {
		t.Fatalf("a silent host must be ConnIdle, got %T %v", err, err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatalf("took %s to notice", time.Since(started))
	}
	if got := Explain(err); got != "the Calabrass host stopped responding" {
		t.Fatalf("message = %q", got)
	}
}

func TestHeartbeatsKeepALongSilentToolAlive(t *testing.T) {
	oldFloor, oldCheck, oldBeats := idleFloor, idleCheck, idleBeatsMax
	idleFloor, idleCheck, idleBeatsMax = 300*time.Millisecond, 20*time.Millisecond, 0
	defer func() { idleFloor, idleCheck, idleBeatsMax = oldFloor, oldCheck, oldBeats }()

	c := sseServer(t, func(w http.ResponseWriter, flush func()) {
		fmt.Fprint(w, ": calabrass-heartbeat 1\n\n")
		flush()
		for i := 0; i < 8; i++ { // 800 ms of "tool running", but beating
			time.Sleep(100 * time.Millisecond)
			fmt.Fprint(w, ": keep-alive\n\n")
			flush()
		}
		fmt.Fprint(w, "event: done\ndata: {\"message\":{}}\n\n")
	})
	if _, err := send(c, context.Background()); err != nil {
		t.Fatalf("a host that is beating was cut off: %v", err)
	}
}

func TestAnOlderHostThatNeverBeatsIsNeverCutOff(t *testing.T) {
	oldFloor, oldCheck, oldBeats := idleFloor, idleCheck, idleBeatsMax
	idleFloor, idleCheck, idleBeatsMax = 50*time.Millisecond, 10*time.Millisecond, 0
	defer func() { idleFloor, idleCheck, idleBeatsMax = oldFloor, oldCheck, oldBeats }()

	c := sseServer(t, func(w http.ResponseWriter, flush func()) {
		flush()
		time.Sleep(400 * time.Millisecond) // silent, and never promised to beat
		fmt.Fprint(w, "event: done\ndata: {\"message\":{}}\n\n")
	})
	if _, err := send(c, context.Background()); err != nil {
		t.Fatalf("a host that never promised heartbeats must not be timed out: %v", err)
	}
}

func TestNothingFromAServerCanDriveTheTerminal(t *testing.T) {
	c := sseServer(t, func(w http.ResponseWriter, flush func()) {
		fmt.Fprint(w, "event: token\ndata: {\"text\":\"ok \\u001b]52;c;ZXZpbA==\\u0007done\"}\n\n")
		fmt.Fprint(w, "event: reasoning\ndata: {\"text\":\"\\u001b[2Jthink\"}\n\n")
		fmt.Fprint(w, "event: trace\ndata: {\"trace\":{\"steps\":[{\"number\":1,\"parts\":[{\"kind\":\"tool\",\"name\":\"run\\u001b[31m\",\"output\":\"\\u001b]0;pwned\\u0007out\"}]}]}}\n\n")
		fmt.Fprint(w, "event: done\ndata: {\"message\":{\"content\":\"x\\u001b[1my\"}}\n\n")
	})
	events, err := send(c, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, e := range events {
		all.WriteString(e.Text)
		if e.Trace != nil {
			for _, p := range e.Trace.Steps[0].Parts {
				all.WriteString(p.Name + p.Output)
			}
		}
		if e.Done != nil {
			all.WriteString(e.Done.Message.Content)
		}
	}
	if strings.ContainsRune(all.String(), 0x1b) || strings.ContainsRune(all.String(), 7) {
		t.Fatalf("a control character reached the UI: %q", all.String())
	}
	if !strings.Contains(all.String(), "ok done") || !strings.Contains(all.String(), "out") {
		t.Fatalf("readable text was lost: %q", all.String())
	}
}

func TestAWebPageInPlaceOfTheHostIsABadReplyNotMarkupOnScreen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, "<html><body><h1>502 Bad Gateway</h1><script>alert(1)</script></body></html>")
	}))
	defer srv.Close()
	err := New(srv.URL).Health(context.Background())
	if !IsBadReply(err) {
		t.Fatalf("got %T %v", err, err)
	}
	if strings.Contains(Explain(err), "<") || strings.Contains(Explain(err), "script") {
		t.Fatalf("markup reached the message: %q", Explain(err))
	}

	// 200 with a body that is not JSON (another program on the port).
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "SSH-2.0-OpenSSH\r\n")
	}))
	defer srv2.Close()
	if _, err := New(srv2.URL).Chats(context.Background()); !IsBadReply(err) {
		t.Fatalf("a non-JSON 200 must be a bad reply, got %T %v", err, err)
	}
}

func TestAnAPIErrorMessageIsCleanedAndClipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":"bad \u001b[31m%s"}`, strings.Repeat("x", 2000))
	}))
	defer srv.Close()
	err := New(srv.URL).Health(context.Background())
	if !IsAPIError(err) || strings.ContainsRune(err.Error(), 0x1b) || len(err.Error()) > 450 {
		t.Fatalf("%q", err.Error())
	}
}

func TestAnEnormousLineIsRefusedNotBuffered(t *testing.T) {
	var written atomic.Int64
	c := sseServer(t, func(w http.ResponseWriter, flush func()) {
		fmt.Fprint(w, "data: ")
		chunk := strings.Repeat("a", 1<<20)
		for i := 0; i < maxLine/(1<<20)+4; i++ {
			n, err := io.WriteString(w, chunk)
			written.Add(int64(n))
			if err != nil {
				return
			}
		}
	})
	_, err := send(c, context.Background())
	if err == nil {
		t.Fatal("an endless line was accepted")
	}
}
