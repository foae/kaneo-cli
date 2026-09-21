package main

import (
	"strings"
	"testing"
)

func TestIndexCommandsRejectsDuplicateSourceMapping(t *testing.T) {
	mapping := commandFile{
		SchemaVersion: 1,
		Operations: []commandEntry{
			{Method: "GET", Path: "/task", Group: "task", Action: "list", Status: "planned"},
			{Method: "GET", Path: "/task", Group: "task", Action: "get", Status: "planned"},
		},
	}
	_, _, err := indexCommands(mapping, map[string]bool{"planned": true})
	if err == nil || !strings.Contains(err.Error(), "duplicate command mapping") {
		t.Fatalf("indexCommands() error = %v, want duplicate source mapping error", err)
	}
}

func TestCollectOperationsRejectsMissingMapping(t *testing.T) {
	paths := map[string]any{
		"/task": map[string]any{
			"get": map[string]any{
				"responses": map[string]any{"200": map[string]any{"description": "ok"}},
			},
		},
	}
	_, err := collectOperations(paths, map[string]commandEntry{}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing command mapping") {
		t.Fatalf("collectOperations() error = %v, want missing mapping error", err)
	}
}

func TestRenderBodyHelpResolvesReferencesAndNestedRequirements(t *testing.T) {
	renderer := bodyHelpRenderer{
		document: map[string]any{
			"components": map[string]any{
				"schemas": map[string]any{
					"CreateTask": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"items": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"priority": map[string]any{"type": "string", "enum": []any{"low", "high"}},
										"title":    map[string]any{"type": "string", "description": "Visible task title"},
									},
									"required": []any{"title"},
								},
							},
						},
						"required": []any{"items"},
					},
				},
			},
		},
		resolving: make(map[string]bool),
	}

	help, err := renderer.renderBodyHelp(map[string]any{"$ref": "#/components/schemas/CreateTask"}, true)
	if err != nil {
		t.Fatalf("renderBodyHelp() error = %v", err)
	}
	for _, expected := range []string{
		"items (required): array<object>",
		"items: object",
		"title (required): string — Visible task title",
		"priority (optional): string (one of: \"low\", \"high\")",
	} {
		if !strings.Contains(help, expected) {
			t.Errorf("rendered help does not contain %q:\n%s", expected, help)
		}
	}
}

func TestRenderBodyHelpShowsAllOfAlternatives(t *testing.T) {
	renderer := bodyHelpRenderer{resolving: make(map[string]bool)}
	help, err := renderer.renderBodyHelp(map[string]any{
		"allOf": []any{
			map[string]any{
				"type":       "object",
				"properties": map[string]any{"organizationId": map[string]any{"type": "string"}},
			},
			map[string]any{
				"anyOf": []any{
					map[string]any{"type": "object", "properties": map[string]any{"roleId": map[string]any{"type": "string"}}, "required": []any{"roleId"}},
					map[string]any{"type": "object", "properties": map[string]any{"roleName": map[string]any{"type": "string"}}, "required": []any{"roleName"}},
				},
			},
		},
	}, true)
	if err != nil {
		t.Fatalf("renderBodyHelp() error = %v", err)
	}
	for _, expected := range []string{"organizationId (optional): string", "one of:", "option 1:", "roleId (required): string", "option 2:", "roleName (required): string"} {
		if !strings.Contains(help, expected) {
			t.Errorf("rendered help does not contain %q:\n%s", expected, help)
		}
	}
}
