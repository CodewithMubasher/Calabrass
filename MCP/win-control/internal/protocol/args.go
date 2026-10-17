package protocol

import (
	"fmt"
	"strings"
)

// RequiredString reads a string argument the caller must provide.
func RequiredString(args map[string]any, name string, maxLen int) (string, error) {
	value, ok := args[name].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required, and must be something to act on", name)
	}
	return clean(value, name, maxLen)
}

// OptionalString reads a string argument, or the fallback when it was left out.
func OptionalString(args map[string]any, name, fallback string, maxLen int) (string, error) {
	value, ok := args[name].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	return clean(value, name, maxLen)
}

// clean trims a value and refuses one that is too long to be a real request, or
// that carries control characters: everything these tools do ends up typed or
// pasted somewhere, and a stray control character is a mistake, not a request.
func clean(value, name string, maxLen int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s must not be empty", name)
	}
	if len(value) > maxLen {
		return "", fmt.Errorf("%s is too long: keep it under %d characters", name, maxLen)
	}
	for _, r := range value {
		if (r < 0x20 && r != '\t') || r == 0x7f {
			return "", fmt.Errorf("%s must not contain control characters", name)
		}
	}
	return value, nil
}
