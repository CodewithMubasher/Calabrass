package protocol

// The argument helpers. A tool is described once, in JSON Schema, and read once,
// through these: the description is what the model is shown, and the reader is
// what keeps a malformed argument from reaching a spreadsheet.

// Object is the input schema for a tool: its properties, and which of them the
// model must provide.
func Object(properties map[string]any, required ...string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// String is a property holding text.
func String(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

// Choice is a property holding one of a known set of words.
func Choice(description string, values ...string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
		"enum":        values,
	}
}

// Integer is a property holding a whole number, with the bounds made explicit
// so a model is not left to guess what a row number may be.
func Integer(description string, minimum, maximum int) map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": description,
		"minimum":     minimum,
		"maximum":     maximum,
	}
}

// Number is a property holding any number.
func Number(description string) map[string]any {
	return map[string]any{"type": "number", "description": description}
}

// Boolean is a property holding true or false.
func Boolean(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

// Array is a property holding a list of text.
func Array(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       map[string]any{"type": "string"},
	}
}

// Grid is a property holding a grid of values: an array of rows, each row an
// array of cells. The cells are deliberately untyped, because a spreadsheet
// cell may be text, a number, a date or a formula.
func Grid(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type":  "array",
			"items": map[string]any{},
		},
	}
}

// Style is a property holding a formatting object, described property by
// property so the model knows what it may set.
func Style(description string) map[string]any {
	return map[string]any{
		"type":        "object",
		"description": description,
		"properties": map[string]any{
			"bold":          Boolean("Show the text heavier."),
			"italic":        Boolean("Slant the text."),
			"underline":     Choice("Underline the text.", "none", "single", "double"),
			"strike":        Boolean("Draw a line through the text."),
			"font":          String("Font name, e.g. \"Calibri\" or \"Arial\"."),
			"font_size":     Number("Font size in points."),
			"font_color":    String("Text colour as a hex value, e.g. \"#1F4E78\" or \"FFFFFF\"."),
			"fill_color":    String("Cell background as a hex value, e.g. \"#DDEBF7\"."),
			"number_format": String("A preset name — general, integer, decimal, currency, percent, date, datetime, time, text, scientific — or a raw Excel format code such as \"#,##0.00 kg\"."),
			"horizontal":    Choice("Where the text sits across the cell.", "left", "center", "right", "fill", "justify"),
			"vertical":      Choice("Where the text sits down the cell.", "top", "center", "bottom", "justify"),
			"wrap":          Boolean("Wrap long text onto more lines."),
			"rotation":      Integer("Rotate the text this many degrees, -90 to 90.", -90, 90),
			"indent":        Integer("Indent the text this many steps.", 0, 15),
			"border":        Choice("Draw a border on all four sides.", "none", "thin", "medium", "dashed", "dotted", "thick", "double", "hair", "mediumDashed", "dashDot", "mediumDashDot", "dashDotDot", "mediumDashDotDot", "slantDashDot"),
			"border_color":  String("Border colour as a hex value."),
		},
	}
}
