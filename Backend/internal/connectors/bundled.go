package connectors

import (
	"os"
	"path/filepath"
	"strings"
)

// Bundled is one of the MCP servers that ships with the app: the name it is
// shown under, and the command that starts it.
type Bundled struct {
	Name    string
	Command string
}

// bundledServers are the servers the installer can put beside the executable.
// The names are the ones the interface has icons for, and each one is packed in
// the folder of the same name.
var bundledServers = []struct {
	name   string
	folder string
	binary string
}{
	{name: "excel", folder: "excel", binary: "excel-mcp.exe"},
	{name: "win-control", folder: "win-control", binary: "win-control-mcp.exe"},
}

// BundledAt returns the bundled MCP servers actually present under root, each
// with an absolute command, so a connector keeps working no matter which
// directory the app was started from. A server the installer left out is not
// there, and so is never offered.
func BundledAt(root string) []Bundled {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}

	found := make([]Bundled, 0, len(bundledServers))
	for _, server := range bundledServers {
		path := filepath.Join(root, "MCP", server.folder, server.binary)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		found = append(found, Bundled{Name: server.name, Command: path})
	}
	return found
}
