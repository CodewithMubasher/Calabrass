package store

import (
	"context"
	"path/filepath"
	"testing"
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
