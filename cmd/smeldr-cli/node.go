package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

// fieldFlag accumulates repeated --field K=V options into a map, preserving the
// key's case exactly (block Fields keys are case-sensitive PascalCase — unlike
// content frontmatter, which is merged case-insensitively).
type fieldFlag map[string]any

func (f fieldFlag) String() string { return "" }

func (f fieldFlag) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok {
		return fmt.Errorf("expected Key=Value, got %q", v)
	}
	if k == "" {
		return fmt.Errorf("field key must not be empty in %q", v)
	}
	f[k] = val
	return nil
}

// runNodeCommand dispatches `block node` verbs. args begins with the verb.
func runNodeCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: smeldr-cli block node <verb> [args]")
		fmt.Fprintln(os.Stderr, "Verbs: create update get list publish archive")
		os.Exit(1)
	}
	switch args[0] {
	case "-h", "--help", "help":
		printBlockHelp(os.Stdout)
	case "create":
		runNodeCreate(args[1:])
	case "update":
		runNodeUpdate(args[1:])
	case "get":
		runNodeByID(args[1:], "get_node", "get")
	case "publish":
		runNodeByID(args[1:], "publish_node", "publish")
	case "archive":
		runNodeByID(args[1:], "archive_node", "archive")
	case "list":
		runNodeList(args[1:])
	default:
		fatal("unknown node verb %q — use: create update get list publish archive", args[0])
	}
}

// buildFields merges a --fields JSON object (base) with --field K=V overrides,
// preserving key case. Either source may be empty.
func buildFields(fieldsJSON string, fields fieldFlag) (map[string]any, error) {
	base := map[string]any{}
	if strings.TrimSpace(fieldsJSON) != "" {
		if err := json.Unmarshal([]byte(fieldsJSON), &base); err != nil {
			return nil, fmt.Errorf("--fields must be a JSON object: %w", err)
		}
	}
	for k, v := range fields {
		base[k] = v
	}
	return base, nil
}

// runNodeCreate creates a Draft block via the create_node MCP tool.
func runNodeCreate(args []string) {
	fs := flag.NewFlagSet("block node create", flag.ExitOnError)
	typeName := fs.String("type", "", "block type_name (required), e.g. content_block, hero")
	fieldsJSON := fs.String("fields", "", "type-specific fields as a JSON object (PascalCase keys)")
	fields := fieldFlag{}
	fs.Var(fields, "field", "a field as Key=Value, repeatable (keys are case-sensitive PascalCase)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: smeldr-cli block node create --type <type_name> [--field K=V ...] [--fields <json>]")
		fs.PrintDefaults()
	}
	fs.Parse(args) //nolint:errcheck

	if *typeName == "" {
		fatal("block node create requires --type <type_name>")
	}
	merged, err := buildFields(*fieldsJSON, fields)
	if err != nil {
		fatal("%v", err)
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "create_node", map[string]any{"type_name": *typeName, "fields": merged})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// runNodeUpdate partially updates a block's fields via update_node. The id is the
// first positional argument; flags follow it.
func runNodeUpdate(args []string) {
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		fatal("block node update requires <id> (smeldr-cli block node update <id> [--field K=V ...] [--fields <json>])")
	}
	id := args[0]

	fs := flag.NewFlagSet("block node update", flag.ExitOnError)
	fieldsJSON := fs.String("fields", "", "fields to merge as a JSON object (PascalCase keys)")
	fields := fieldFlag{}
	fs.Var(fields, "field", "a field as Key=Value, repeatable")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: smeldr-cli block node update <id> [--field K=V ...] [--fields <json>]")
		fs.PrintDefaults()
	}
	fs.Parse(args[1:]) //nolint:errcheck

	merged, err := buildFields(*fieldsJSON, fields)
	if err != nil {
		fatal("%v", err)
	}
	if len(merged) == 0 {
		fatal("block node update requires at least one --field or --fields")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "update_node", map[string]any{"id": id, "fields": merged})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// runNodeByID runs a single-argument node tool (get_node / publish_node /
// archive_node) on the block whose id is the first positional argument.
func runNodeByID(args []string, tool, verb string) {
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		fatal("block node %s requires <id>", verb)
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, tool, map[string]any{"id": args[0]})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// runNodeList lists blocks via list_nodes, printing an aligned table by default
// or raw JSON with --json.
func runNodeList(args []string) {
	fs := flag.NewFlagSet("block node list", flag.ExitOnError)
	typeName := fs.String("type", "", "filter by block type_name")
	status := fs.String("status", "", "filter by status (draft|scheduled|published|archived)")
	asJSON := fs.Bool("json", false, "output raw JSON instead of a table")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: smeldr-cli block node list [--type <type_name>] [--status <s>] [--json]")
		fs.PrintDefaults()
	}
	fs.Parse(args) //nolint:errcheck

	toolArgs := map[string]any{}
	if *typeName != "" {
		toolArgs["type_name"] = *typeName
	}
	if *status != "" {
		toolArgs["status"] = *status
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "list_nodes", toolArgs)
	if err != nil {
		fatal("%v", err)
	}
	if *asJSON {
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
		return
	}
	table, err := nodeListTable(text)
	if err != nil {
		fatal("%v", err)
	}
	fmt.Print(table)
}

// nodeListTable renders a list_nodes JSON result as an aligned table with columns
// ID, TYPE_NAME, STATUS, SLUG. An empty list yields a short notice.
func nodeListTable(jsonText string) (string, error) {
	var resp struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(jsonText), &resp); err != nil {
		return "", fmt.Errorf("decode list_nodes response: %w", err)
	}
	if len(resp.Items) == 0 {
		return "No nodes.\n", nil
	}
	rows := make([][]string, 0, len(resp.Items))
	for _, it := range resp.Items {
		rows = append(rows, []string{
			asString(it["ID"]),
			asString(it["type_name"]),
			asString(it["Status"]),
			asString(it["Slug"]),
		})
	}
	return renderTable([]string{"ID", "TYPE_NAME", "STATUS", "SLUG"}, rows), nil
}

// asString renders a decoded JSON value as a plain string for table cells.
func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// renderTable formats headers and rows as a left-aligned, space-padded table
// with a dashed separator row. Column widths fit the widest cell.
func renderTable(headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if i < len(widths) && len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}

	var b strings.Builder
	writeRow := func(cells []string) {
		for i, c := range cells {
			b.WriteString(c)
			if i < len(cells)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-len(c)+2))
			}
		}
		b.WriteString("\n")
	}

	writeRow(headers)
	seps := make([]string, len(headers))
	for i := range seps {
		seps[i] = strings.Repeat("-", widths[i])
	}
	writeRow(seps)
	for _, r := range rows {
		writeRow(r)
	}
	return b.String()
}
