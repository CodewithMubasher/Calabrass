package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// pointTheDataFolderAtTemp redirects the per-user folder into a temporary one,
// so a test can see where the app would write without touching the real folder.
func pointTheDataFolderAtTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LOCALAPPDATA", dir)
	case "darwin":
		t.Setenv("HOME", dir)
	default:
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
	return dir
}

// nothingConfigured removes the overrides, so a developer's own environment
// cannot decide what these tests see. They are unset rather than emptied,
// because an empty variable counts as set and would stop a .env file being read.
func nothingConfigured(t *testing.T) {
	t.Helper()
	for _, key := range []string{"CALABRASS_DB", "CALABRASS_WORKSPACES_DIR", "CALABRASS_WORKSPACE"} {
		previous, existed := os.LookupEnv(key)
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(key, previous)
				return
			}
			_ = os.Unsetenv(key)
		})
		_ = os.Unsetenv(key)
	}
}

// The app's own data belongs to the user, not to the project it was built from.
// A clean of the checkout must never be able to reach someone's chats or the
// files they were working on.
func TestUserDataLivesOutsideTheProject(t *testing.T) {
	root := pointTheDataFolderAtTemp(t)
	nothingConfigured(t)

	cfg := LoadFrom(filepath.Join(t.TempDir(), ".env"))

	for name, path := range map[string]string{
		"DatabasePath":  cfg.DatabasePath,
		"WorkspacesDir": cfg.WorkspacesDir,
	} {
		if !filepath.IsAbs(path) {
			t.Errorf("%s = %q, want an absolute path", name, path)
			continue
		}
		if !strings.HasPrefix(path, root) {
			t.Errorf("%s = %q, want it inside the user data folder %q", name, path, root)
		}
	}
}

// A relative path in the .env means "inside my data folder", not "inside
// whichever directory the process was started from" — the older behaviour that
// put the database back inside the checkout.
func TestRelativeOverridesResolveInsideTheDataFolder(t *testing.T) {
	pointTheDataFolderAtTemp(t)
	nothingConfigured(t)

	env := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(env, []byte("CALABRASS_DB=data/calabrass.db\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := LoadFrom(env)
	want := filepath.Join(DataDir(), "data", "calabrass.db")
	if cfg.DatabasePath != want {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, want)
	}
}

// An absolute path is still honoured as written: an explicit setting is the
// user's to make.
func TestAbsoluteOverridesAreHonoured(t *testing.T) {
	pointTheDataFolderAtTemp(t)
	nothingConfigured(t)

	chosen := filepath.Join(t.TempDir(), "elsewhere.db")
	t.Setenv("CALABRASS_DB", chosen)

	if cfg := LoadFrom(filepath.Join(t.TempDir(), ".env")); cfg.DatabasePath != chosen {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, chosen)
	}
}
