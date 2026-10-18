package protocol

import (
	"encoding/json"
	"fmt"
	"math"
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

// StringList reads a list of strings, refusing one that is not a list at all.
func StringList(args map[string]any, name string, maxItems int) ([]string, error) {
	raw, ok := args[name]
	if !ok || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		if list, ok := raw.([]string); ok {
			return list, nil
		}
		return nil, fmt.Errorf("%s must be a list of text", name)
	}
	if maxItems > 0 && len(items) > maxItems {
		return nil, fmt.Errorf("%s has %d items: keep it under %d", name, len(items), maxItems)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be a list of text", name)
		}
		out = append(out, strings.TrimSpace(text))
	}
	return out, nil
}

// RequiredInt reads a whole-number argument the caller must provide.
func RequiredInt(args map[string]any, name string, minimum, maximum int) (int, error) {
	value, ok := args[name]
	if !ok || value == nil {
		return 0, fmt.Errorf("%s is required", name)
	}
	number, err := whole(value, name)
	if err != nil {
		return 0, err
	}
	if number < minimum || number > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum)
	}
	return number, nil
}

// OptionalInt reads a whole-number argument, or the fallback when it was left
// out. Zero is a real value here, so it is only the absence that falls back.
func OptionalInt(args map[string]any, name string, fallback, minimum, maximum int) (int, error) {
	value, ok := args[name]
	if !ok || value == nil {
		return fallback, nil
	}
	number, err := whole(value, name)
	if err != nil {
		return 0, err
	}
	if number < minimum || number > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum)
	}
	return number, nil
}

// OptionalFloat reads a number, or the fallback when it was left out.
func OptionalFloat(args map[string]any, name string, fallback float64) (float64, error) {
	value, ok := args[name]
	if !ok || value == nil {
		return fallback, nil
	}
	number, err := decimal(value, name)
	if err != nil {
		return 0, err
	}
	return number, nil
}

// OptionalBool reads a true/false argument, or the fallback when it was left
// out. A JSON boolean is expected; the words "true" and "false" are accepted
// too, because a model that wrote the value into a string meant the same thing.
func OptionalBool(args map[string]any, name string, fallback bool) (bool, error) {
	value, ok := args[name]
	if !ok || value == nil {
		return fallback, nil
	}
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "yes", "1":
			return true, nil
		case "false", "no", "0":
			return false, nil
		}
	}
	return false, fmt.Errorf("%s must be true or false", name)
}

// RequiredChoice reads a string argument that must be one of a known set. Each
// value carries its own description in the schema, so the tool's help is where
// the model looks first; this is the second line of defence, and its message
// names the words that would have worked.
func RequiredChoice(args map[string]any, name string, values ...string) (string, error) {
	raw, err := RequiredString(args, name, 100)
	if err != nil {
		return "", err
	}
	chosen := strings.ToLower(raw)
	for _, value := range values {
		if chosen == strings.ToLower(value) {
			return value, nil
		}
	}
	return "", fmt.Errorf("%s is %q: use one of %s", name, raw, strings.Join(values, ", "))
}

// OptionalChoice reads a string argument from a known set, or the fallback when
// it was left out.
func OptionalChoice(args map[string]any, name, fallback string, values ...string) (string, error) {
	raw, err := OptionalString(args, name, "", 100)
	if err != nil {
		return "", err
	}
	if raw == "" {
		return fallback, nil
	}
	chosen := strings.ToLower(raw)
	for _, value := range values {
		if chosen == strings.ToLower(value) {
			return value, nil
		}
	}
	return "", fmt.Errorf("%s is %q: use one of %s", name, raw, strings.Join(values, ", "))
}

// Rows reads a grid of cell values: a list of rows, each row a list of cells.
// The reader is where a model's shape mistake is caught, before it reaches a
// worksheet — a row that is not a list, or a cell that is an object, is refused
// with the shape that was expected rather than written somewhere unreadable.
func Rows(args map[string]any, name string, maxRows, maxCols int) ([][]any, error) {
	raw, ok := args[name]
	if !ok || raw == nil {
		return nil, fmt.Errorf("%s is required, and must be a list of rows", name)
	}
	rows, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list of rows, each row a list of cell values", name)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s is empty: give at least one row", name)
	}
	if maxRows > 0 && len(rows) > maxRows {
		return nil, fmt.Errorf("%s has %d rows: keep it under %d", name, len(rows), maxRows)
	}

	out := make([][]any, 0, len(rows))
	for index, raw := range rows {
		row, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("%s row %d is not a list of cell values", name, index+1)
		}
		if maxCols > 0 && len(row) > maxCols {
			return nil, fmt.Errorf("%s row %d has %d cells: keep rows under %d", name, index+1, len(row), maxCols)
		}
		for _, cell := range row {
			if err := checkCell(cell, name, index+1); err != nil {
				return nil, err
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// checkCell refuses the values a spreadsheet cell cannot hold: an object or a
// list inside a cell is a shape mistake, not a value.
func checkCell(cell any, name string, row int) error {
	switch cell.(type) {
	case nil, string, bool, float64, json.Number, int, int64:
		return nil
	default:
		return fmt.Errorf("%s row %d has a cell that is not text, a number, true/false or null", name, row)
	}
}

// whole reads a value that must be a whole number.
func whole(value any, name string) (int, error) {
	number, err := decimal(value, name)
	if err != nil {
		return 0, err
	}
	if math.Trunc(number) != number {
		return 0, fmt.Errorf("%s must be a whole number", name)
	}
	return int(number), nil
}

// decimal reads a value that must be a number.
func decimal(value any, name string) (float64, error) {
	switch typed := value.(type) {
	case float64:
		return typed, nil
	case int:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case json.Number:
		number, err := typed.Float64()
		if err != nil {
			return 0, fmt.Errorf("%s must be a number", name)
		}
		return number, nil
	case string:
		var number json.Number = json.Number(strings.TrimSpace(typed))
		parsed, err := number.Float64()
		if err != nil {
			return 0, fmt.Errorf("%s must be a number", name)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s must be a number", name)
	}
}

// clean trims a value and refuses one that is too long to be a real request, or
// that carries control characters: everything these tools do ends up inside a
// workbook, and a stray control character is a mistake, not a request.
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
