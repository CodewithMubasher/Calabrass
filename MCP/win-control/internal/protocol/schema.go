package protocol

// The argument helpers. A tool is described once, in JSON Schema, and read once,
// through these: the description is what the model is shown, and the reader is
// what keeps a malformed argument from reaching the desktop.

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
