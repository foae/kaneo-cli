package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// commandStatus is one reviewed source-to-command mapping.
type commandStatus struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Group  string `json:"group"`
	Action string `json:"action"`
	Status string `json:"status"`
}

// TestInventoryCoverageIsHonest reconciles the reviewed mapping against the
// registered command tree. Every implemented command has a RunE.
func TestInventoryCoverageIsHonest(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "commands.json"))
	if err != nil {
		t.Fatalf("read commands: %v", err)
	}
	var doc struct {
		Operations []commandStatus `json:"operations"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse commands: %v", err)
	}
	if len(doc.Operations) == 0 {
		t.Fatal("no operations in mapping")
	}

	root := newTestEnv(t).app.newRootCommand()

	seen := make(map[string]bool)
	for _, entry := range doc.Operations {
		key := entry.Method + " " + entry.Path
		if seen[key] {
			t.Errorf("duplicate mapping for %s", key)
		}
		seen[key] = true

		if entry.Status != "implemented" {
			t.Errorf("%s %s: mapped command %s %s is not implemented (status %q)",
				entry.Method, entry.Path, entry.Group, entry.Action, entry.Status)
			continue
		}

		group := findCommand(root, entry.Group)
		if group == nil {
			t.Errorf("%s %s: group %q is not registered", entry.Method, entry.Path, entry.Group)
			continue
		}
		command := findCommand(group, entry.Action)
		if command == nil {
			t.Errorf("%s %s: implemented command %s %s is not registered", entry.Method, entry.Path, entry.Group, entry.Action)
			continue
		}
		if command.RunE == nil {
			t.Errorf("%s %s: implemented command has no RunE", entry.Method, entry.Path)
		}
	}
}
