package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// The two modes used to be "build" and "plan". Plan is gone and build is the
// ordinary chat, so a database written by the earlier build has to come back
// saying "chat" — otherwise its conversations would run under a mode this
// build does not know.
func TestOldModesBecomeChat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calabrass.db")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	ctx := context.Background()
	for _, mode := range []string{"build", "plan"} {
		chat, err := st.CreateChat(ctx, "p", "m", "", "", mode)
		if err != nil {
			t.Fatalf("create chat: %v", err)
		}
		if chat.Mode != mode {
			t.Fatalf("the chat was stored in mode %q, want %q", chat.Mode, mode)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	// Open it again: this is where the migration runs.
	st, err = Open(path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	chats, err := st.ListChats(ctx)
	if err != nil {
		t.Fatalf("list chats: %v", err)
	}
	if len(chats) != 2 {
		t.Fatalf("the list has %d chats", len(chats))
	}
	for _, chat := range chats {
		if chat.Mode != ModeChat {
			t.Errorf("the chat came back in mode %q, want %q", chat.Mode, ModeChat)
		}
	}
}

// A compacted turn used to be written as `["summary": "..."]`, which is not
// JSON. The chat reader then had to drop it, and anything that encoded the row
// directly — an API response, for instance — failed on it. Compaction must
// leave valid JSON in both columns, and the replay must still read as a
// provider turn so the next model call can use the summary.
func TestCompactedMessagesKeepValidJSON(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	chat, err := st.CreateChat(ctx, "p", "m", "", "", "")
	if err != nil {
		t.Fatalf("create chat: %v", err)
	}

	const trace = `{"steps":[{"parts":[{"kind":"tool","name":"delegate_task"}]}]}`
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		msg, err := st.AddMessage(ctx, Message{
			ChatID:  chat.ID,
			Role:    "assistant",
			Content: "the work is done",
			Trace:   json.RawMessage(trace),
		})
		if err != nil {
			t.Fatalf("add message %d: %v", i, err)
		}
		ids = append(ids, msg.ID)
	}

	// Order the turns explicitly: AddMessage can write all three in the same
	// millisecond, and the cutoff is chosen by created_at.
	base := time.Now().UnixMilli() - 10_000
	for i, id := range ids {
		if _, err := st.db.ExecContext(ctx,
			`UPDATE messages SET created_at = ? WHERE id = ?`, base+int64(i), id,
		); err != nil {
			t.Fatalf("order message %d: %v", i, err)
		}
	}

	n, err := st.CompactMessages(ctx, chat.ID, 1)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if n != 1 {
		t.Fatalf("compacted %d messages, want 1", n)
	}

	messages, err := st.ListMessages(ctx, chat.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}

	var compacted *Message
	for i := range messages {
		if messages[i].Compacted {
			compacted = &messages[i]
		}
	}
	if compacted == nil {
		t.Fatal("no message was marked compacted")
	}
	if !json.Valid(compacted.Trace) {
		t.Errorf("trace is not valid JSON: %s", compacted.Trace)
	}
	if !json.Valid(compacted.Replay) {
		t.Errorf("replay is not valid JSON: %s", compacted.Replay)
	}

	var replayed []map[string]any
	if err := json.Unmarshal(compacted.Replay, &replayed); err != nil {
		t.Fatalf("replay does not parse as a provider transcript: %v", err)
	}
	if len(replayed) != 1 {
		t.Fatalf("replay has %d turns, want 1", len(replayed))
	}
	if replayed[0]["Role"] != "assistant" {
		t.Errorf("replay role is %v, want assistant", replayed[0]["Role"])
	}
}

// A chat with no mode at all is an ordinary one.
func TestCreateChatDefaultsToChat(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "calabrass.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	chat, err := st.CreateChat(context.Background(), "p", "m", "", "", "")
	if err != nil {
		t.Fatalf("create chat: %v", err)
	}
	if chat.Mode != ModeChat {
		t.Fatalf("the chat was stored in mode %q, want %q", chat.Mode, ModeChat)
	}
}
