//go:build !(windows && (amd64 || arm64))

package wininput

// Type does nothing here, because there is nothing here to type into.
func Type(string) error { return ErrUnsupported }

// Tap does nothing here, for the same reason.
func Tap(uint16) error { return ErrUnsupported }
