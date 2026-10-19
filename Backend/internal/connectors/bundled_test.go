package connectors

import (
	"os"
	"path/filepath"
	"testing"
)

// The install directory decides what the app may offer. These tests hold that
// to two rules: a server is offered only when its binary is really there, and
// what is offered is an absolute path, because an installed app is started from
// wherever Windows happens to put it.

func TestBundledAtFindsOnlyWhatIsInstalled(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "MCP", "excel", "excel-mcp.exe"))
	writeFile(t, filepath.Join(root, "MCP", "win-control", "win-control-mcp.exe"))
	// Left over from a run with more servers packed, or an unfinished copy.
	writeFile(t, filepath.Join(root, "MCP", "gmail", "gmail-mcp.exe"))

	found := BundledAt(root)
	if len(found) != 2 {
		t.Fatalf("found %d servers (%v), want the two that ship", len(found), found)
	}
	if found[0].Name != "excel" || found[1].Name != "win-control" {
		t.Errorf("names = %q, %q, want excel then win-control", found[0].Name, found[1].Name)
	}
	// gmail is not one this app installs, so its binary on disk changes nothing.
	for _, server := range found {
		if server.Name == "gmail" {
			t.Error("a server packed by someone else's script should not be offered")
		}
	}
}

func TestBundledAtOffersAnAbsoluteCommand(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "MCP", "excel", "excel-mcp.exe")
	writeFile(t, want)

	found := BundledAt(root)
	if len(found) != 1 {
		t.Fatalf("found %d servers (%v), want one", len(found), found)
	}
	if !filepath.IsAbs(found[0].Command) {
		t.Errorf("command = %q, want an absolute path", found[0].Command)
	}
	if _, err := os.Stat(found[0].Command); err != nil {
		t.Errorf("the command offered does not exist: %v", err)
	}
}

// TestBundledAtSkipsWhatTheInstallerLeftOut: a person who unticked the MCP
// servers during setup must not be shown connectors they cannot run.
func TestBundledAtSkipsWhatTheInstallerLeftOut(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "MCP", "win-control", "win-control-mcp.exe"))

	found := BundledAt(root)
	if len(found) != 1 || found[0].Name != "win-control" {
		t.Fatalf("found %v, want only win-control", found)
	}
}

func TestBundledAtFindsNothingWithoutAnInstallDirectory(t *testing.T) {
	if found := BundledAt(""); found != nil {
		t.Errorf("found %v with no root, want nothing", found)
	}
	if found := BundledAt(t.TempDir()); len(found) != 0 {
		t.Errorf("found %v in a folder with no MCP servers, want nothing", found)
	}
}
