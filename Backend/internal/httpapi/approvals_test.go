package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/auth"
	"github.com/CodewithMubasher/Calabrass/backend/internal/permission"
)

// approvalsServer is a server with only what the approval endpoints need: no
// database, no providers.
func approvalsServer() *Server {
	return &Server{broker: permission.NewBroker()}
}

func asUser(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userContextKey{}, auth.User{ID: id}))
}

func raise(t *testing.T, s *Server, user, command string) <-chan error {
	t.Helper()
	sess := s.interactiveSession(user, "chat-1", "/work/project", nil)
	done := make(chan error, 1)
	go func() {
		done <- sess.Authorize(context.Background(), permission.ClassifyCommand(command, "", "/work/project"))
	}()
	deadline := time.Now().Add(2 * time.Second)
	for len(s.pendingApprovals(user)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no approval was raised")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return done
}

func TestApprovalsAreListedAndAnswered(t *testing.T) {
	s := approvalsServer()
	done := raise(t, s, "u1", "npm test")

	rec := httptest.NewRecorder()
	s.handleListApprovals(rec, asUser(httptest.NewRequest("GET", "/api/approvals", nil), "u1"))
	var list struct {
		Approvals []permission.Request `json:"approvals"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil || len(list.Approvals) != 1 {
		t.Fatalf("list: %v %+v", err, list)
	}
	req := list.Approvals[0]
	if req.Tier != permission.TierExecute || !strings.Contains(req.Summary, "npm test") {
		t.Fatalf("unexpected request %+v", req)
	}

	body := strings.NewReader(`{"decision":"allow_once"}`)
	r := httptest.NewRequest("POST", "/api/approvals/"+req.ID, body)
	r.SetPathValue("approvalID", req.ID)
	rec = httptest.NewRecorder()
	s.handleDecideApproval(rec, asUser(r, "u1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("decide: %d %s", rec.Code, rec.Body.String())
	}
	if err := <-done; err != nil {
		t.Fatalf("the approved action was refused: %v", err)
	}
}

func TestAnotherAccountCannotAnswer(t *testing.T) {
	s := approvalsServer()
	done := raise(t, s, "u1", "npm test")
	id := s.pendingApprovals("u1")[0].ID

	r := httptest.NewRequest("POST", "/api/approvals/"+id, strings.NewReader(`{"decision":"allow_once"}`))
	r.SetPathValue("approvalID", id)
	rec := httptest.NewRecorder()
	s.handleDecideApproval(rec, asUser(r, "intruder"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("another account got %d, want 404", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.handleListApprovals(rec, asUser(httptest.NewRequest("GET", "/api/approvals", nil), "intruder"))
	if strings.Contains(rec.Body.String(), id) {
		t.Fatal("another account can see the approval")
	}

	// The owner refuses it, which also lets the goroutine finish.
	r = httptest.NewRequest("POST", "/api/approvals/"+id, strings.NewReader(`{"decision":"deny"}`))
	r.SetPathValue("approvalID", id)
	s.handleDecideApproval(httptest.NewRecorder(), asUser(r, "u1"))
	if err := <-done; err == nil {
		t.Fatal("a refused action was allowed")
	}
}

func TestADecisionMustBeOneWeKnow(t *testing.T) {
	s := approvalsServer()
	done := raise(t, s, "u1", "npm test")
	id := s.pendingApprovals("u1")[0].ID

	r := httptest.NewRequest("POST", "/api/approvals/"+id, strings.NewReader(`{"decision":"sure"}`))
	r.SetPathValue("approvalID", id)
	rec := httptest.NewRecorder()
	s.handleDecideApproval(rec, asUser(r, "u1"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("nonsense answer got %d, want 400", rec.Code)
	}

	r = httptest.NewRequest("POST", "/api/approvals/"+id, strings.NewReader(`{"decision":"deny"}`))
	r.SetPathValue("approvalID", id)
	s.handleDecideApproval(httptest.NewRecorder(), asUser(r, "u1"))
	<-done
}

func TestSessionForIsCautiousWhenNobodyAskedForOne(t *testing.T) {
	s := approvalsServer()
	sess := s.sessionFor(context.Background(), "u1", "c1", "/work/project")
	if sess.Mode != permission.Unattended {
		t.Fatal("a turn built without a session must be unattended")
	}
	if err := sess.Authorize(context.Background(), permission.ClassifyCommand("npm test", "", "/work/project")); err == nil {
		t.Fatal("an unattended turn with no scope ran a command")
	}
}

func TestSessionForKeepsWhatTheHandlerSet(t *testing.T) {
	s := approvalsServer()
	in := s.interactiveSession("u1", "c1", "", nil)
	ctx := permission.WithSession(context.Background(), in)
	got := s.sessionFor(ctx, "u1", "c1", "/work/project")
	if got.Mode != permission.Interactive || got.Workspace != "/work/project" {
		t.Fatalf("got %+v", got)
	}
}

func TestForgetClearsRemembered(t *testing.T) {
	s := approvalsServer()
	done := raise(t, s, "u1", "npm test")
	id := s.pendingApprovals("u1")[0].ID
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"decision":"allow_run"}`))
	r.SetPathValue("approvalID", id)
	s.handleDecideApproval(httptest.NewRecorder(), asUser(r, "u1"))
	<-done

	sess := s.interactiveSession("u1", "chat-1", "/work/project", nil)
	if err := sess.Authorize(context.Background(), permission.ClassifyCommand("make", "", "/work/project")); err != nil {
		t.Fatalf("allow_run was not remembered: %v", err)
	}
	s.handleForgetApprovals(httptest.NewRecorder(), asUser(httptest.NewRequest("DELETE", "/", nil), "u1"))
	s.broker.Timeout = 20 * time.Millisecond
	if err := sess.Authorize(context.Background(), permission.ClassifyCommand("make", "", "/work/project")); err == nil {
		t.Fatal("forgetting did not clear the remembered answer")
	}
}

func TestSSEWriterRefusesWritesAfterClose(t *testing.T) {
	rec := httptest.NewRecorder()
	stream, err := newSSEWriter(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.event("a", 1); err != nil {
		t.Fatal(err)
	}
	stream.close()
	if err := stream.event("b", 2); err == nil {
		t.Fatal("a write after close was accepted")
	}
	if strings.Contains(rec.Body.String(), "event: b") {
		t.Fatal("a closed stream was written to")
	}
}
