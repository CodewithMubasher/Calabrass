package workbook

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// This file turns the formatting object a model sent into a style the library
// understands, and the other way for the few things a sheet reports back.

// numberFormats are the names a model may use instead of writing an Excel
// format code. Naming them is what keeps "currency" from meaning six different
// strings across six calls.
var numberFormats = map[string]string{
	"integer":    "#,##0",
	"decimal":    "#,##0.00",
	"currency":   `"$"#,##0.00`,
	"percent":    "0.00%",
	"date":       "yyyy-mm-dd",
	"datetime":   "yyyy-mm-dd hh:mm",
	"time":       "hh:mm:ss",
	"text":       "@",
	"scientific": "0.00E+00",
}

// borderStyles maps a border's name to the line the library draws.
var borderStyles = map[string]int{
	"none":             0,
	"thin":             1,
	"medium":           2,
	"dashed":           3,
	"dotted":           4,
	"thick":            5,
	"double":           6,
	"hair":             7,
	"mediumdashed":     8,
	"dashdot":          9,
	"mediumdashdot":    10,
	"dashdotdot":       11,
	"mediumdashdotdot": 12,
	"slantdashdot":     13,
}

// NumberFormatCode resolves a preset name or a raw format code. The second
// result is empty for "general", which is the absence of a format rather than a
// format that says nothing.
func NumberFormatCode(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", nil
	}
	if code, known := numberFormats[strings.ToLower(trimmed)]; known {
		return code, nil
	}
	// Anything else is treated as a format code the caller wrote out, because
	// the codes are a language of their own and refusing an unfamiliar one would
	// block the cases this server did not think of.
	if strings.EqualFold(trimmed, "general") {
		return "", nil
	}
	return trimmed, nil
}

// StyleFrom builds a style from the formatting object a tool received. An empty
// or absent object is refused, because a call that changes nothing is a mistake
// worth naming.
func StyleFrom(spec map[string]any) (*excelize.Style, error) {
	if len(spec) == 0 {
		return nil, fmt.Errorf("a style is required: name at least one of bold, fill_color, number_format, horizontal, wrap or border")
	}

	style := &excelize.Style{}

	font := &excelize.Font{}
	wantsFont := false
	if bold, ok := spec["bold"]; ok {
		value, err := specBool(bold, "bold")
		if err != nil {
			return nil, err
		}
		font.Bold = value
		wantsFont = true
	}
	if italic, ok := spec["italic"]; ok {
		value, err := specBool(italic, "italic")
		if err != nil {
			return nil, err
		}
		font.Italic = value
		wantsFont = true
	}
	if strike, ok := spec["strike"]; ok {
		value, err := specBool(strike, "strike")
		if err != nil {
			return nil, err
		}
		font.Strike = value
		wantsFont = true
	}
	if underline, ok := spec["underline"]; ok {
		text, err := specString(underline, "underline")
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(text, "none") {
			font.Underline = strings.ToLower(text)
			wantsFont = true
		}
	}
	if name, ok := spec["font"]; ok {
		text, err := specString(name, "font")
		if err != nil {
			return nil, err
		}
		font.Family = text
		wantsFont = true
	}
	if size, ok := spec["font_size"]; ok {
		number, err := specNumber(size, "font_size")
		if err != nil {
			return nil, err
		}
		if number <= 0 || number > 400 {
			return nil, fmt.Errorf("font_size must be between 1 and 400")
		}
		font.Size = number
		wantsFont = true
	}
	if color, ok := spec["font_color"]; ok {
		text, err := specString(color, "font_color")
		if err != nil {
			return nil, err
		}
		hex, err := HexColor(text, "font_color")
		if err != nil {
			return nil, err
		}
		font.Color = hex
		wantsFont = true
	}
	if wantsFont {
		style.Font = font
	}

	if fill, ok := spec["fill_color"]; ok {
		text, err := specString(fill, "fill_color")
		if err != nil {
			return nil, err
		}
		hex, err := HexColor(text, "fill_color")
		if err != nil {
			return nil, err
		}
		style.Fill = excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{hex}}
	}

	if format, ok := spec["number_format"]; ok {
		text, err := specString(format, "number_format")
		if err != nil {
			return nil, err
		}
		code, err := NumberFormatCode(text)
		if err != nil {
			return nil, err
		}
		if code != "" {
			style.CustomNumFmt = &code
		}
	}

	alignment := &excelize.Alignment{}
	wantsAlignment := false
	if horizontal, ok := spec["horizontal"]; ok {
		text, err := specString(horizontal, "horizontal")
		if err != nil {
			return nil, err
		}
		alignment.Horizontal = strings.ToLower(text)
		wantsAlignment = true
	}
	if vertical, ok := spec["vertical"]; ok {
		text, err := specString(vertical, "vertical")
		if err != nil {
			return nil, err
		}
		alignment.Vertical = strings.ToLower(text)
		wantsAlignment = true
	}
	if wrap, ok := spec["wrap"]; ok {
		value, err := specBool(wrap, "wrap")
		if err != nil {
			return nil, err
		}
		alignment.WrapText = value
		wantsAlignment = true
	}
	if rotation, ok := spec["rotation"]; ok {
		number, err := specNumber(rotation, "rotation")
		if err != nil {
			return nil, err
		}
		if number < -90 || number > 90 {
			return nil, fmt.Errorf("rotation must be between -90 and 90 degrees")
		}
		alignment.TextRotation = int(number)
		wantsAlignment = true
	}
	if indent, ok := spec["indent"]; ok {
		number, err := specNumber(indent, "indent")
		if err != nil {
			return nil, err
		}
		if number < 0 || number > 15 {
			return nil, fmt.Errorf("indent must be between 0 and 15")
		}
		alignment.Indent = int(number)
		wantsAlignment = true
	}
	if wantsAlignment {
		style.Alignment = alignment
	}

	if border, ok := spec["border"]; ok {
		text, err := specString(border, "border")
		if err != nil {
			return nil, err
		}
		line, known := borderStyles[strings.ToLower(text)]
		if !known {
			return nil, fmt.Errorf("border is %q: use none, thin, medium, dashed, dotted, thick, double or hair", text)
		}
		color := "000000"
		if raw, ok := spec["border_color"]; ok {
			named, err := specString(raw, "border_color")
			if err != nil {
				return nil, err
			}
			if color, err = HexColor(named, "border_color"); err != nil {
				return nil, err
			}
		}
		if line != 0 {
			for _, side := range []string{"left", "right", "top", "bottom"} {
				style.Border = append(style.Border, excelize.Border{Type: side, Style: line, Color: color})
			}
		}
	}

	return style, nil
}

// HexColor normalises a colour a model sent: a leading "#" is a habit, and the
// library wants the six or eight digits without it.
func HexColor(value, name string) (string, error) {
	hex := strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(hex) != 6 && len(hex) != 8 {
		return "", fmt.Errorf("%s must be a hex colour such as \"#1F4E78\"", name)
	}
	for _, r := range hex {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return "", fmt.Errorf("%s must be a hex colour such as \"#1F4E78\"", name)
		}
	}
	return strings.ToUpper(hex), nil
}

func specString(value any, name string) (string, error) {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%s must be text", name)
	}
	return strings.TrimSpace(text), nil
}

func specBool(value any, name string) (bool, error) {
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

func specNumber(value any, name string) (float64, error) {
	switch typed := value.(type) {
	case float64:
		return typed, nil
	case int:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case string:
		var number float64
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%g", &number); err != nil {
			return 0, fmt.Errorf("%s must be a number", name)
		}
		return number, nil
	default:
		return 0, fmt.Errorf("%s must be a number", name)
	}
}
