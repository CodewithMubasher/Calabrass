package httpapi

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
)

func testServer(t *testing.T) (*Server, *agent.Workspace, *agent.Library) {
	t.Helper()

	library, err := agent.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatalf("open library: %v", err)
	}
	def, err := library.Ensure(agent.DefaultWorkspaceName)
	if err != nil {
		t.Fatalf("open default workspace: %v", err)
	}
	return &Server{library: library, workspace: def, log: slog.Default()}, def, library
}

// A chat bound to a workspace has to work in that workspace's folder, inside the
// library, whether or not the folder exists yet. A chat with no binding, or one
// bound to the default, works in the server's default workspace.
func TestWorkspaceForUsesTheChatsFolder(t *testing.T) {
	s, def, library := testServer(t)

	bound := s.workspaceFor(store.Chat{ID: "1", WorkspaceDir: "landing-page"})
	if got, want := filepath.Base(bound.Root()), "landing-page"; got != want {
		t.Fatalf("workspace folder = %q, want %q", got, want)
	}
	if !strings.HasPrefix(bound.Root(), library.Root()) {
		t.Fatalf("workspace %s is outside the library %s", bound.Root(), library.Root())
	}
	if info, err := os.Stat(bound.Root()); err != nil || !info.IsDir() {
		t.Fatalf("workspace folder was not created: %v", err)
	}

	for _, chat := range []store.Chat{
		{ID: "2"},
		{ID: "3", WorkspaceDir: agent.DefaultWorkspaceName},
	} {
		if got := s.workspaceFor(chat); got != def {
			t.Fatalf("chat %s: workspace = %s, want the default %s", chat.ID, got.Root(), def.Root())
		}
	}
}

// A binding that would climb out of the library cannot be opened, so the turn
// falls back to the default rather than escaping.
func TestWorkspaceForRefusesEscapingFolders(t *testing.T) {
	s, def, _ := testServer(t)

	for _, dir := range []string{"..", "../outside", `C:\elsewhere`, "a/b", ""} {
		if got := s.workspaceFor(store.Chat{ID: "1", WorkspaceDir: dir}); got != def {
			t.Fatalf("workspace %q: chat escaped to %s", dir, got.Root())
		}
	}
}
