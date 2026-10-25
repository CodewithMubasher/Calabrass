package settings

import (
	"os"
	"runtime"
	"testing"
)

func TestRoundTripAndPermissions(t *testing.T) {
	t.Setenv(EnvDir, t.TempDir())

	got, err := Load()
	if err != nil || got != (Settings{}) {
		t.Fatalf("missing file should load empty, got %+v %v", got, err)
	}

	want := Settings{Addr: "127.0.0.1:1", Token: "t", Provider: "p", Model: "m"}
	if err := want.Save(); err != nil {
		t.Fatal(err)
	}
	got, err = Load()
	if err != nil || got != want {
		t.Fatalf("round trip: %+v %v", got, err)
	}

	if runtime.GOOS != "windows" {
		path, _ := Path()
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o600 {
			t.Errorf("settings file mode = %v, want 0600", info.Mode().Perm())
		}
	}
}
