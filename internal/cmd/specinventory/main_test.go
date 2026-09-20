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
