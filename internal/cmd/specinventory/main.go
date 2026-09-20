// specinventory builds the reviewed API operation inventory from vendored inputs.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	openAPIPath    = "api/openapi.json"
	commandsPath   = "api/commands.json"
	provenancePath = "api/provenance.json"
	outputPath     = "api/operations.json"
	documentPath   = "docs/api/operations.md"
)

var operationMethods = map[string]bool{
	"delete":  true,
	"get":     true,
	"head":    true,
	"options": true,
	"patch":   true,
	"post":    true,
	"put":     true,
	"trace":   true,
}

var commandName = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

type commandFile struct {
	SchemaVersion int            `json:"schema_version"`
	StatusValues  []string       `json:"status_values"`
	Operations    []commandEntry `json:"operations"`
}

type commandEntry struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Group  string `json:"group"`
	Action string `json:"action"`
	Status string `json:"status"`
}

type provenance struct {
	OpenAPI struct {
		SHA256 string `json:"sha256"`
	} `json:"openapi"`
}

type generatedInventory struct {
	SchemaVersion  int                  `json:"schema_version"`
	OperationCount int                  `json:"operation_count"`
	Source         generatedSource      `json:"source"`
	Operations     []generatedOperation `json:"operations"`
}

type generatedSource struct {
	OpenAPIPath string `json:"openapi_path"`
	SHA256      string `json:"sha256"`
}

type generatedOperation struct {
	Method      string         `json:"method"`
	Path        string         `json:"path"`
	OperationID string         `json:"operation_id,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Description string         `json:"description,omitempty"`
	Command     commandView    `json:"command"`
	Parameters  []any          `json:"parameters,omitempty"`
	RequestBody any            `json:"request_body,omitempty"`
	Security    any            `json:"security,omitempty"`
	Responses   map[string]any `json:"responses"`
}

type commandView struct {
	Group  string `json:"group"`
	Action string `json:"action"`
	Status string `json:"status"`
}

func main() {
	check := flag.Bool("check", false, "fail if generated inventory files are stale")
	flag.Parse()
	if flag.NArg() != 0 {
		fatalf("usage: specinventory [--check]")
	}

	inventory, err := buildInventory()
	if err != nil {
		fatalf("specinventory: %v", err)
	}
	jsonOutput, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		fatalf("marshal inventory: %v", err)
	}
	jsonOutput = append(jsonOutput, '\n')
	documentOutput := renderDocument(inventory)

	outputs := map[string][]byte{outputPath: jsonOutput, documentPath: []byte(documentOutput)}
	for path, content := range outputs {
		if *check {
			current, err := os.ReadFile(path)
			if err != nil || string(current) != string(content) {
				fatalf("generated file is stale: %s (run go run ./internal/cmd/specinventory)", path)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fatalf("create output directory for %s: %v", path, err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			fatalf("write %s: %v", path, err)
		}
	}
}

func buildInventory() (generatedInventory, error) {
	openAPIBytes, err := os.ReadFile(openAPIPath)
	if err != nil {
		return generatedInventory{}, fmt.Errorf("read %s: %w", openAPIPath, err)
	}
	var spec map[string]any
	decoder := json.NewDecoder(bytes.NewReader(openAPIBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&spec); err != nil {
		return generatedInventory{}, fmt.Errorf("parse %s: %w", openAPIPath, err)
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		return generatedInventory{}, errors.New("OpenAPI document has no paths object")
	}

	var mapping commandFile
	if err := readJSON(commandsPath, &mapping); err != nil {
		return generatedInventory{}, err
	}
	if mapping.SchemaVersion != 1 {
		return generatedInventory{}, fmt.Errorf("unsupported commands schema version %d", mapping.SchemaVersion)
	}
	validStatuses := make(map[string]bool, len(mapping.StatusValues))
	for _, status := range mapping.StatusValues {
		if status == "" || validStatuses[status] {
			return generatedInventory{}, fmt.Errorf("invalid or duplicate status %q", status)
		}
		validStatuses[status] = true
	}
	if len(validStatuses) == 0 {
		return generatedInventory{}, errors.New("commands status_values is empty")
	}

	bySource, _, err := indexCommands(mapping, validStatuses)
	if err != nil {
		return generatedInventory{}, err
	}
	operations, err := collectOperations(paths, bySource, spec["security"])
	if err != nil {
		return generatedInventory{}, err
	}
	if len(bySource) != 0 {
		orphans := sortedKeys(bySource)
		return generatedInventory{}, fmt.Errorf("orphan command mappings: %s", strings.Join(orphans, ", "))
	}

	var source provenance
	if err := readJSON(provenancePath, &source); err != nil {
		return generatedInventory{}, err
	}
	sum := sha256.Sum256(openAPIBytes)
	digest := hex.EncodeToString(sum[:])
	if source.OpenAPI.SHA256 == "" {
		return generatedInventory{}, errors.New("provenance has no OpenAPI SHA256")
	}
	if !strings.EqualFold(digest, source.OpenAPI.SHA256) {
		return generatedInventory{}, fmt.Errorf("OpenAPI SHA256 %s does not match provenance %s", digest, source.OpenAPI.SHA256)
	}
	return generatedInventory{
		SchemaVersion:  1,
		OperationCount: len(operations),
		Source:         generatedSource{OpenAPIPath: openAPIPath, SHA256: digest},
		Operations:     operations,
	}, nil
}

func readJSON(path string, destination any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func indexCommands(mapping commandFile, validStatuses map[string]bool) (map[string]commandEntry, map[string]bool, error) {
	bySource := make(map[string]commandEntry, len(mapping.Operations))
	commandNames := make(map[string]bool, len(mapping.Operations))
	for _, entry := range mapping.Operations {
		entry.Method = strings.ToUpper(entry.Method)
		key := operationKey(entry.Method, entry.Path)
		if entry.Method == "" || entry.Path == "" || !strings.HasPrefix(entry.Path, "/") {
			return nil, nil, fmt.Errorf("invalid command mapping %q", key)
		}
		if _, exists := bySource[key]; exists {
			return nil, nil, fmt.Errorf("duplicate command mapping for %s", key)
		}
		if !commandName.MatchString(entry.Group) || !commandName.MatchString(entry.Action) {
			return nil, nil, fmt.Errorf("invalid command name for %s: %q %q", key, entry.Group, entry.Action)
		}
		if !validStatuses[entry.Status] {
			return nil, nil, fmt.Errorf("unknown status %q for %s", entry.Status, key)
		}
		commandKey := entry.Group + " " + entry.Action
		if commandNames[commandKey] {
			return nil, nil, fmt.Errorf("duplicate command name %q", commandKey)
		}
		bySource[key] = entry
		commandNames[commandKey] = true
	}
	return bySource, commandNames, nil
}

func collectOperations(paths map[string]any, bySource map[string]commandEntry, inheritedSecurity any) ([]generatedOperation, error) {
	keys := sortedKeys(paths)
	operations := make([]generatedOperation, 0, len(bySource))
	for _, path := range keys {
		pathItem, ok := paths[path].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path item %q is not an object", path)
		}
		pathParameters := values(pathItem["parameters"])
		methods := sortedKeys(pathItem)
		for _, method := range methods {
			if !operationMethods[method] {
				continue
			}
			operation, ok := pathItem[method].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("operation %s %s is not an object", strings.ToUpper(method), path)
			}
			key := operationKey(strings.ToUpper(method), path)
			entry, found := bySource[key]
			if !found {
				return nil, fmt.Errorf("missing command mapping for %s", key)
			}
			delete(bySource, key)
			parameters := append(append([]any(nil), pathParameters...), values(operation["parameters"])...)
			security := operation["security"]
			if security == nil {
				security = inheritedSecurity
			}
			result := generatedOperation{
				Method:      strings.ToUpper(method),
				Path:        path,
				OperationID: stringValue(operation["operationId"]),
				Tags:        stringValues(operation["tags"]),
				Summary:     stringValue(operation["summary"]),
				Description: stringValue(operation["description"]),
				Command:     commandView{Group: entry.Group, Action: entry.Action, Status: entry.Status},
				Parameters:  parameters,
				RequestBody: operation["requestBody"],
				Security:    security,
				Responses:   objectValues(operation["responses"]),
			}
			if result.Responses == nil {
				return nil, fmt.Errorf("operation %s has no responses object", key)
			}
			operations = append(operations, result)
		}
	}
	sort.Slice(operations, func(i, j int) bool {
		if operations[i].Path == operations[j].Path {
			return operations[i].Method < operations[j].Method
		}
		return operations[i].Path < operations[j].Path
	})
	return operations, nil
}

func renderDocument(inventory generatedInventory) string {
	var document strings.Builder
	document.WriteString("<!-- Code generated by internal/cmd/specinventory; DO NOT EDIT. -->\n\n")
	document.WriteString("# API operation inventory\n\n")
	fmt.Fprintf(&document, "This inventory contains %d operations from [`%s`](../../%s) (SHA-256 `%s`). Every entry is mapped to a planned CLI command; this file does not expose or implement any command.\n\n", inventory.OperationCount, inventory.Source.OpenAPIPath, inventory.Source.OpenAPIPath, inventory.Source.SHA256)
	document.WriteString("Detailed request schemas, query/path parameters, effective security, and responses are in the generated [`api/operations.json`](../../api/operations.json). Schema references resolve against the pinned OpenAPI snapshot.\n\n")
	byGroup := make(map[string][]generatedOperation)
	for _, operation := range inventory.Operations {
		byGroup[operation.Command.Group] = append(byGroup[operation.Command.Group], operation)
	}
	for _, group := range sortedKeys(byGroup) {
		document.WriteString("## " + group + "\n\n")
		document.WriteString("| Command | Method | Path | Operation ID | Status | Parameters | Request body | Responses |\n| --- | --- | --- | --- | --- | ---: | --- | --- |\n")
		operations := byGroup[group]
		sort.Slice(operations, func(i, j int) bool {
			if operations[i].Command.Action == operations[j].Command.Action {
				if operations[i].Path == operations[j].Path {
					return operations[i].Method < operations[j].Method
				}
				return operations[i].Path < operations[j].Path
			}
			return operations[i].Command.Action < operations[j].Command.Action
		})
		for _, operation := range operations {
			body := ""
			if operation.RequestBody != nil {
				body = "yes"
			}
			responses := sortedKeys(operation.Responses)
			fmt.Fprintf(&document, "| `%s %s` | `%s` | `%s` | `%s` | `%s` | %d | %s | %s |\n", operation.Command.Group, operation.Command.Action, operation.Method, operation.Path, operation.OperationID, operation.Command.Status, len(operation.Parameters), body, strings.Join(responses, ", "))
		}
		document.WriteString("\n")
	}
	return document.String()
}

func operationKey(method, path string) string { return method + " " + path }

func values(value any) []any {
	values, _ := value.([]any)
	return values
}

func objectValues(value any) map[string]any {
	values, _ := value.(map[string]any)
	return values
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func stringValues(value any) []string {
	values := values(value)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
