// Package version holds the build's version, set at link time:
//
//	go build -ldflags "-X github.com/CodewithMubasher/Calabrass/backend/internal/version.Version=1.2.0"
package version

// Version is the release this binary was built from.
var Version = "dev"
