package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderBodyHelpRejectsRecursiveSchemaReferences(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]any
	}{
		{
			name: "object property",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"child": map[string]any{"$ref": "#/components/schemas/Node"},
				},
			},
		},
		{
			name: "array item",
			schema: map[string]any{
				"type":  "array",
				"items": map[string]any{"$ref": "#/components/schemas/Node"},
			},
		},
		{
			name: "oneOf branch",
			schema: map[string]any{
				"oneOf": []any{
					map[string]any{"type": "string"},
					map[string]any{"$ref": "#/components/schemas/Node"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			renderer := bodyHelpRenderer{
				document: map[string]any{
					"components": map[string]any{
						"schemas": map[string]any{"Node": test.schema},
					},
				},
				resolving: make(map[string]bool),
			}

			_, err := renderer.renderBodyHelp(map[string]any{"$ref": "#/components/schemas/Node"}, true)
			if err == nil || !strings.Contains(err.Error(), `cyclic schema reference "#/components/schemas/Node"`) {
				t.Fatalf("renderBodyHelp() error = %v, want controlled cyclic reference error", err)
			}
		})
	}
}

func TestRenderBodyHelpInheritsOuterAllOfRequirements(t *testing.T) {
	renderer := bodyHelpRenderer{resolving: make(map[string]bool)}
	help, err := renderer.renderBodyHelp(map[string]any{
		"allOf": []any{
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
			},
		},
		"required": []any{"name"},
	}, true)
	if err != nil {
		t.Fatalf("renderBodyHelp() error = %v", err)
	}
	if !strings.Contains(help, "name (required): string") {
		t.Errorf("rendered help does not contain required allOf property:\n%s", help)
	}
	if strings.Contains(help, "name (optional): string") {
		t.Errorf("rendered help marks outer required allOf property optional:\n%s", help)
	}
}

func TestRenderBodyHelpRendersSchemaBounds(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []any{"content"},
		"properties": map[string]any{
			"content": map[string]any{"type": "string", "minLength": json.Number("1"), "maxLength": json.Number("10000")},
			"labels": map[string]any{
				"type":     "array",
				"items":    map[string]any{"type": "string", "format": "uuid", "maxLength": json.Number("64")},
				"minItems": json.Number("1"),
				"maxItems": json.Number("100"),
			},
			"minutes": map[string]any{"type": "integer", "minimum": json.Number("5"), "maximum": json.Number("1000000"), "default": json.Number("30")},
			"ratio":   map[string]any{"type": "number", "exclusiveMinimum": json.Number("0"), "exclusiveMaximum": json.Number("1.5")},
			"mode":    map[string]any{"type": "string", "enum": []any{"a", "b"}, "maxLength": json.Number("1")},
			"when":    map[string]any{"type": "string", "format": "date-time", "pattern": "^x$"},
		},
	}
	renderer := bodyHelpRenderer{resolving: make(map[string]bool)}
	help, err := renderer.renderBodyHelp(schema, true)
	if err != nil {
		t.Fatalf("renderBodyHelp() error = %v", err)
	}
	for _, want := range []string{
		"content (required): string (min length: 1, max length: 10000)",
		"(min items: 1, max items: 100)",
		"string (format: uuid) (max length: 64)",
		"minutes (optional): integer (minimum: 5, maximum: 1000000) (default: 30)",
		"ratio (optional): number (exclusive minimum: 0, exclusive maximum: 1.5)",
		`mode (optional): string (one of: "a", "b") (max length: 1)`,
		"when (optional): string (format: date-time)",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("rendered help missing %q:\n%s", want, help)
		}
	}
	if strings.Contains(help, "pattern") || strings.Contains(help, "e+") {
		t.Errorf("rendered help contains pattern or float formatting:\n%s", help)
	}
}
