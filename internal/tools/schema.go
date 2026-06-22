package tools

// StringProperty returns a JSON schema string property.
func StringProperty(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

// IntegerProperty returns a JSON schema integer property.
func IntegerProperty(description string, minimum int) map[string]any {
	return map[string]any{
		"type":        "integer",
		"minimum":     minimum,
		"description": description,
	}
}

// ObjectSchema returns a strict JSON object schema.
func ObjectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}
