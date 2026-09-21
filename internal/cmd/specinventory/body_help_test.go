package main

import (
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
