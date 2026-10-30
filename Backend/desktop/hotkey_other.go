//go:build !windows

package main

import "log/slog"

// registerShowHotkey is Windows-only for now: macOS and Linux each need their own
// global-shortcut mechanism, and none is wired yet.
func registerShowHotkey(_ *slog.Logger, _ func()) (stop func()) { return func() {} }
