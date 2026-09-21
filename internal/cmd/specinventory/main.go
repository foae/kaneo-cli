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
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	openAPIPath    = "api/openapi.json"
	commandsPath   = "api/commands.json"
	provenancePath = "api/provenance.json"
	outputPath     = "api/operations.json"
	documentPath   = "docs/api/operations.md"
	bodyHelpPath   = "internal/cli/body_help_generated.go"
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
	BodyHelp       map[string]string    `json:"-"`
	OpenAPIDoc     map[string]any       `json:"-"`
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
	documentOutput, err := renderDocument(inventory)
	if err != nil {
		fatalf("render operation reference: %v", err)
	}
	bodyHelpOutput, err := format.Source([]byte(renderBodyHelpSource(inventory.BodyHelp)))
	if err != nil {
		fatalf("format generated body help: %v", err)
	}

	outputs := map[string][]byte{
		outputPath:   jsonOutput,
		documentPath: []byte(documentOutput),
		bodyHelpPath: bodyHelpOutput,
	}
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
	bodyHelp, err := buildBodyHelp(spec, operations)
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
		BodyHelp:       bodyHelp,
		OpenAPIDoc:     spec,
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

func renderDocument(inventory generatedInventory) (string, error) {
	var document strings.Builder
	document.WriteString("<!-- Code generated by internal/cmd/specinventory; DO NOT EDIT. -->\n\n")
	document.WriteString("# API operation inventory\n\n")
	fmt.Fprintf(&document, "This inventory contains %d operations from [`%s`](../../%s) (SHA-256 `%s`). Each entry is mapped to a CLI command with a coverage status: `implemented` commands have runnable behavior, `planned` commands are design references only.\n\n", inventory.OperationCount, inventory.Source.OpenAPIPath, inventory.Source.OpenAPIPath, inventory.Source.SHA256)
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
		for _, operation := range operations {
			reference, err := renderBodyReference(operation, inventory.BodyHelp, inventory.OpenAPIDoc)
			if err != nil {
				return "", fmt.Errorf("%s: %w", operation.OperationID, err)
			}
			if reference == "" {
				continue
			}
			fmt.Fprintf(&document, "### `%s %s` request\n\n%s", operation.Command.Group, operation.Command.Action, reference)
		}
	}
	return document.String(), nil
}

var dedicatedUploadReferences = map[string]string{
	"createTaskImageUpload": "Dedicated upload contract: pass the local image with `--file`; the CLI obtains a presigned storage destination, uploads the file without API credentials, and finalizes the image against the task.",
	"uploadUserAvatar":      "Dedicated upload contract: pass the local avatar with `--file`; the CLI reads the local file and sends its detected content type and base64 data as the documented avatar request.",
}

type bodyHelpRenderer struct {
	document  map[string]any
	resolving map[string]bool
}

func buildBodyHelp(document map[string]any, operations []generatedOperation) (map[string]string, error) {
	renderer := bodyHelpRenderer{document: document, resolving: make(map[string]bool)}
	help := make(map[string]string)
	for _, operation := range operations {
		if _, dedicated := dedicatedUploadReferences[operation.OperationID]; dedicated {
			continue
		}
		body := objectValues(operation.RequestBody)
		if body == nil {
			continue
		}
		content := objectValues(body["content"])
		jsonContent := objectValues(content["application/json"])
		schema := objectValues(jsonContent["schema"])
		if schema == nil {
			return nil, fmt.Errorf("operation %s request body has no application/json schema", operation.OperationID)
		}
		text, err := renderer.renderBodyHelp(schema, boolValue(body["required"]))
		if err != nil {
			return nil, fmt.Errorf("render request body for %s: %w", operation.OperationID, err)
		}
		help[operation.OperationID] = text
	}
	return help, nil
}

func (r *bodyHelpRenderer) renderBodyHelp(schema map[string]any, required bool) (string, error) {
	if err := r.validateSchemaReferences(schema, make(map[string]bool)); err != nil {
		return "", err
	}
	fields, err := r.renderObjectContents(schema, "  ", nil)
	if err != nil {
		return "", err
	}
	if len(fields) == 0 {
		summary, err := r.schemaSummary(schema)
		if err != nil {
			return "", err
		}
		fields = []string{"  value: " + summary}
	}
	requirement := "optional"
	if required {
		requirement = "required"
	}
	return "JSON body (" + requirement + "):\n" + strings.Join(fields, "\n"), nil
}

func (r *bodyHelpRenderer) renderObjectContents(raw map[string]any, indent string, inheritedRequired map[string]bool) ([]string, error) {
	schema, err := r.resolveSchema(raw)
	if err != nil {
		return nil, err
	}
	var lines []string
	required := requiredProperties(schema)
	for name := range inheritedRequired {
		required[name] = true
	}
	for _, member := range values(schema["allOf"]) {
		component := objectValues(member)
		if component == nil {
			return nil, errors.New("allOf member is not a schema object")
		}
		componentLines, err := r.renderObjectContents(component, indent, required)
		if err != nil {
			return nil, err
		}
		lines = append(lines, componentLines...)
	}
	properties := objectValues(schema["properties"])
	for _, name := range sortedKeys(properties) {
		property := objectValues(properties[name])
		if property == nil {
			return nil, fmt.Errorf("property %q is not a schema object", name)
		}
		propertyLines, err := r.renderProperty(name, property, required[name], indent)
		if err != nil {
			return nil, err
		}
		lines = append(lines, propertyLines...)
	}
	alternatives := values(schema["anyOf"])
	if len(alternatives) > 0 {
		lines = append(lines, indent+"one of:")
		for index, alternative := range alternatives {
			alternativeSchema := objectValues(alternative)
			if alternativeSchema == nil {
				return nil, errors.New("anyOf member is not a schema object")
			}
			lines = append(lines, fmt.Sprintf("%s  option %d:", indent, index+1))
			alternativeLines, err := r.renderObjectContents(alternativeSchema, indent+"    ", nil)
			if err != nil {
				return nil, err
			}
			if len(alternativeLines) == 0 {
				summary, err := r.schemaSummary(alternativeSchema)
				if err != nil {
					return nil, err
				}
				lines = append(lines, indent+"    value: "+summary)
			} else {
				lines = append(lines, alternativeLines...)
			}
		}
	}
	if additional, found := schema["additionalProperties"]; found {
		summary, err := r.additionalPropertiesSummary(additional)
		if err != nil {
			return nil, err
		}
		lines = append(lines, indent+"additional properties: "+summary)
	}
	return lines, nil
}

func (r *bodyHelpRenderer) renderProperty(name string, raw map[string]any, required bool, indent string) ([]string, error) {
	schema, err := r.resolveSchema(raw)
	if err != nil {
		return nil, err
	}
	summary, err := r.schemaSummary(schema)
	if err != nil {
		return nil, err
	}
	requirement := "optional"
	if required {
		requirement = "required"
	}
	line := fmt.Sprintf("%s%s (%s): %s", indent, name, requirement, summary)
	if description := inlineDescription(stringValue(schema["description"])); description != "" {
		line += " — " + description
	}
	lines := []string{line}
	if isObjectSchema(schema) {
		children, err := r.renderObjectContents(schema, indent+"  ", nil)
		if err != nil {
			return nil, err
		}
		lines = append(lines, children...)
	}
	if isArraySchema(schema) {
		item := objectValues(schema["items"])
		if item == nil {
			return nil, fmt.Errorf("array property %q has no item schema", name)
		}
		itemSummary, err := r.schemaSummary(item)
		if err != nil {
			return nil, err
		}
		lines = append(lines, indent+"  items: "+itemSummary)
		itemSchema, err := r.resolveSchema(item)
		if err != nil {
			return nil, err
		}
		if isObjectSchema(itemSchema) {
			children, err := r.renderObjectContents(itemSchema, indent+"    ", nil)
			if err != nil {
				return nil, err
			}
			lines = append(lines, children...)
		}
	}
	return lines, nil
}

func (r *bodyHelpRenderer) schemaSummary(raw map[string]any) (string, error) {
	schema, err := r.resolveSchema(raw)
	if err != nil {
		return "", err
	}
	types, err := schemaTypes(schema["type"])
	if err != nil {
		return "", err
	}
	var summary string
	if len(types) > 0 {
		summary = strings.Join(types, " or ")
		if isArraySchema(schema) {
			item := objectValues(schema["items"])
			if item == nil {
				return "", errors.New("array schema has no item schema")
			}
			itemSummary, err := r.schemaSummary(item)
			if err != nil {
				return "", err
			}
			arraySummary := "array<" + itemSummary + ">"
			nonArrayTypes := make([]string, 0, len(types)-1)
			for _, schemaType := range types {
				if schemaType != "array" {
					nonArrayTypes = append(nonArrayTypes, schemaType)
				}
			}
			if len(nonArrayTypes) == 0 {
				summary = arraySummary
			} else {
				summary = arraySummary + " or " + strings.Join(nonArrayTypes, " or ")
			}
		}
	} else if len(values(schema["anyOf"])) > 0 {
		alternatives := make([]string, 0, len(values(schema["anyOf"])))
		for _, alternative := range values(schema["anyOf"]) {
			alternativeSchema := objectValues(alternative)
			if alternativeSchema == nil {
				return "", errors.New("anyOf member is not a schema object")
			}
			alternativeSummary, err := r.schemaSummary(alternativeSchema)
			if err != nil {
				return "", err
			}
			alternatives = append(alternatives, alternativeSummary)
		}
		summary = "one of: " + strings.Join(alternatives, " | ")
	} else if isObjectSchema(schema) {
		summary = "object"
	} else {
		return "", errors.New("schema has no supported type")
	}
	if enum := values(schema["enum"]); len(enum) > 0 {
		enumValues := make([]string, 0, len(enum))
		for _, value := range enum {
			encoded, err := json.Marshal(value)
			if err != nil {
				return "", fmt.Errorf("marshal enum value: %w", err)
			}
			enumValues = append(enumValues, string(encoded))
		}
		summary += " (one of: " + strings.Join(enumValues, ", ") + ")"
	}
	if format := stringValue(schema["format"]); format != "" {
		summary += " (format: " + format + ")"
	}
	if value, found := schema["default"]; found {
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("marshal default value: %w", err)
		}
		summary += " (default: " + string(encoded) + ")"
	}
	return summary, nil
}

func (r *bodyHelpRenderer) additionalPropertiesSummary(value any) (string, error) {
	if schema := objectValues(value); schema != nil {
		if len(schema) == 0 {
			return "any value", nil
		}
		summary, err := r.schemaSummary(schema)
		if err != nil {
			return "", err
		}
		return summary, nil
	}
	if allowed, ok := value.(bool); ok {
		if allowed {
			return "any value", nil
		}
		return "not allowed", nil
	}
	return "", errors.New("additionalProperties is not a schema or boolean")
}

func (r *bodyHelpRenderer) resolveSchema(schema map[string]any) (map[string]any, error) {
	reference := stringValue(schema["$ref"])
	if reference == "" {
		return schema, nil
	}
	if r.resolving[reference] {
		return nil, fmt.Errorf("cyclic schema reference %q", reference)
	}
	r.resolving[reference] = true
	defer delete(r.resolving, reference)
	target, err := r.resolveReference(reference)
	if err != nil {
		return nil, err
	}
	resolved, err := r.resolveSchema(target)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]any, len(resolved)+len(schema))
	for key, value := range resolved {
		merged[key] = value
	}
	for key, value := range schema {
		if key != "$ref" {
			merged[key] = value
		}
	}
	return merged, nil
}

func (r *bodyHelpRenderer) validateSchemaReferences(schema map[string]any, references map[string]bool) error {
	reference := stringValue(schema["$ref"])
	if reference != "" {
		if references[reference] {
			return fmt.Errorf("cyclic schema reference %q", reference)
		}
		references[reference] = true
		target, err := r.resolveReference(reference)
		if err != nil {
			return err
		}
		if err := r.validateSchemaReferences(target, references); err != nil {
			return err
		}
		delete(references, reference)
	}
	for _, property := range objectValues(schema["properties"]) {
		if propertySchema := objectValues(property); propertySchema != nil {
			if err := r.validateSchemaReferences(propertySchema, references); err != nil {
				return err
			}
		}
	}
	for _, member := range values(schema["allOf"]) {
		if memberSchema := objectValues(member); memberSchema != nil {
			if err := r.validateSchemaReferences(memberSchema, references); err != nil {
				return err
			}
		}
	}
	for _, member := range values(schema["anyOf"]) {
		if memberSchema := objectValues(member); memberSchema != nil {
			if err := r.validateSchemaReferences(memberSchema, references); err != nil {
				return err
			}
		}
	}
	if item := objectValues(schema["items"]); item != nil {
		if err := r.validateSchemaReferences(item, references); err != nil {
			return err
		}
	}
	if additionalProperties := objectValues(schema["additionalProperties"]); additionalProperties != nil {
		if err := r.validateSchemaReferences(additionalProperties, references); err != nil {
			return err
		}
	}
	return nil
}

func (r *bodyHelpRenderer) resolveReference(reference string) (map[string]any, error) {
	if reference == "#" {
		return r.document, nil
	}
	if !strings.HasPrefix(reference, "#/") {
		return nil, fmt.Errorf("unsupported non-local schema reference %q", reference)
	}
	var current any = r.document
	for _, escapedPart := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
		part := strings.NewReplacer("~1", "/", "~0", "~").Replace(escapedPart)
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("schema reference %q traverses a non-object", reference)
		}
		var found bool
		current, found = object[part]
		if !found {
			return nil, fmt.Errorf("schema reference %q does not exist", reference)
		}
	}
	schema := objectValues(current)
	if schema == nil {
		return nil, fmt.Errorf("schema reference %q is not an object", reference)
	}
	return schema, nil
}

func renderBodyReference(operation generatedOperation, bodyHelp map[string]string, document map[string]any) (string, error) {
	if dedicated, ok := dedicatedUploadReferences[operation.OperationID]; ok {
		return dedicated + "\n\n", nil
	}
	text, ok := bodyHelp[operation.OperationID]
	if !ok {
		return "", nil
	}
	renderer := bodyHelpRenderer{document: document, resolving: make(map[string]bool)}
	var reference strings.Builder
	pathParameters, err := renderer.renderPathParameters(operation.Parameters)
	if err != nil {
		return "", err
	}
	if len(pathParameters) > 0 {
		reference.WriteString("Path parameters (not JSON body fields):\n")
		reference.WriteString(strings.Join(pathParameters, "\n"))
		reference.WriteString("\n\n")
	}
	reference.WriteString("```text\n")
	reference.WriteString(text)
	reference.WriteString("\n```\n\n")
	return reference.String(), nil
}

func (r *bodyHelpRenderer) renderPathParameters(parameters []any) ([]string, error) {
	var lines []string
	for _, parameter := range parameters {
		value := objectValues(parameter)
		if value == nil {
			continue
		}
		value, err := r.resolveSchema(value)
		if err != nil {
			return nil, err
		}
		if stringValue(value["in"]) != "path" {
			continue
		}
		name := stringValue(value["name"])
		if name == "" {
			return nil, errors.New("path parameter has no name")
		}
		schema := objectValues(value["schema"])
		if schema == nil {
			return nil, fmt.Errorf("path parameter %q has no schema", name)
		}
		summary, err := r.schemaSummary(schema)
		if err != nil {
			return nil, fmt.Errorf("path parameter %q: %w", name, err)
		}
		requirement := "optional"
		if boolValue(value["required"]) {
			requirement = "required"
		}
		line := fmt.Sprintf("- `%s` (%s): %s", name, requirement, summary)
		if description := inlineDescription(stringValue(value["description"])); description != "" {
			line += " — " + description
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func renderBodyHelpSource(bodyHelp map[string]string) string {
	var source strings.Builder
	source.WriteString("// Code generated by internal/cmd/specinventory; DO NOT EDIT.\n\n")
	source.WriteString("package cli\n\n")
	source.WriteString("var generatedBodyHelp = map[string]string{\n")
	for _, operationID := range sortedKeys(bodyHelp) {
		fmt.Fprintf(&source, "\t%s: %s,\n", strconv.Quote(operationID), strconv.Quote(bodyHelp[operationID]))
	}
	source.WriteString("}\n")
	return source.String()
}

func requiredProperties(schema map[string]any) map[string]bool {
	required := make(map[string]bool)
	for _, name := range stringValues(schema["required"]) {
		required[name] = true
	}
	return required
}

func schemaTypes(value any) ([]string, error) {
	switch value := value.(type) {
	case nil:
		return nil, nil
	case string:
		return []string{value}, nil
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				return nil, errors.New("schema type array contains a non-string")
			}
			result = append(result, text)
		}
		return result, nil
	default:
		return nil, errors.New("schema type is not a string or string array")
	}
}

func isObjectSchema(schema map[string]any) bool {
	if len(objectValues(schema["properties"])) > 0 || len(values(schema["allOf"])) > 0 || schema["additionalProperties"] != nil {
		return true
	}
	types, err := schemaTypes(schema["type"])
	if err != nil {
		return false
	}
	for _, schemaType := range types {
		if schemaType == "object" {
			return true
		}
	}
	return false
}

func isArraySchema(schema map[string]any) bool {
	types, err := schemaTypes(schema["type"])
	if err != nil {
		return false
	}
	for _, schemaType := range types {
		if schemaType == "array" {
			return true
		}
	}
	return false
}

func inlineDescription(description string) string {
	return strings.Join(strings.Fields(description), " ")
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
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
