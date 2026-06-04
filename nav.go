package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// runNavCommand dispatches nav subcommands. args begins with the verb.
func runNavCommand(args []string) {
	if len(args) == 0 {
		printNavHelp()
		os.Exit(1)
	}
	switch args[0] {
	case "-h", "--help", "help":
		printNavHelp()
	case "list":
		runNavList(args[1:])
	case "create":
		runNavCreate(args[1:])
	case "update":
		runNavUpdate(args[1:])
	case "delete":
		runNavDelete(args[1:])
	default:
		fatal("unknown nav verb %q — use: list create update delete", args[0])
	}
}

func printNavHelp() {
	fmt.Fprint(os.Stdout, `forge-cli nav — navigation tree management (Editor role required)

Verbs:
  list                                         list all navigation items
  list --json                                  list items as JSON
  create --label <label> [flags]               create a navigation item
  update <id> [flags]                          update a navigation item
  delete <id>                                  delete an item and all its descendants

Flags for create / update:
  --label <text>       display label (required for create)
  --path <path>        URL path prefix, e.g. /learn
  --parent-id <id>     ID of parent item (top-level if omitted)
  --module <name>      module table name this item maps to
  --hidden             exclude from navigation, keep in breadcrumbs
  --ghost              non-clickable structural grouping node
  --sort-order <n>     display order within parent (lower = earlier)

Writes require the server to be in DB nav mode. list works in any nav mode.
The MCP endpoint is used for all nav operations (SMELDR_MCP_URL).
`)
}

// runNavList lists all navigation items via list_nav_items MCP tool.
func runNavList(args []string) {
	var jsonFlag bool
	for _, a := range args {
		if a == "--json" {
			jsonFlag = true
		}
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}

	text, err := mcpCall(cfg, "list_nav_items", map[string]any{})
	if err != nil {
		fatal("%v", err)
	}
	if jsonFlag {
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
		return
	}
	table, err := navListTable(text)
	if err != nil {
		fatal("%v", err)
	}
	fmt.Print(table)
}

// runNavCreate creates a navigation item via create_nav_item MCP tool.
func runNavCreate(args []string) {
	var label, path, parentID, module string
	var hidden, ghost bool
	var sortOrder int

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--label":
			if i+1 < len(args) {
				label = args[i+1]
				i++
			}
		case "--path":
			if i+1 < len(args) {
				path = args[i+1]
				i++
			}
		case "--parent-id":
			if i+1 < len(args) {
				parentID = args[i+1]
				i++
			}
		case "--module":
			if i+1 < len(args) {
				module = args[i+1]
				i++
			}
		case "--hidden":
			hidden = true
		case "--ghost":
			ghost = true
		case "--sort-order":
			if i+1 < len(args) {
				n, err := strconv.Atoi(args[i+1])
				if err != nil {
					fatal("--sort-order must be a number: %v", err)
				}
				sortOrder = n
				i++
			}
		}
	}
	if label == "" {
		fatal("nav create requires --label <label>")
	}

	callArgs := map[string]any{
		"label":      label,
		"path":       path,
		"parent_id":  parentID,
		"module":     module,
		"hidden":     hidden,
		"ghost":      ghost,
		"sort_order": float64(sortOrder),
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "create_nav_item", callArgs)
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// runNavUpdate updates a navigation item via update_nav_item MCP tool.
// The first argument is the item ID; subsequent arguments are optional field flags.
func runNavUpdate(args []string) {
	if len(args) == 0 {
		fatal("nav update requires an item ID")
	}
	id := args[0]
	args = args[1:]

	callArgs := map[string]any{"id": id}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--label":
			if i+1 < len(args) {
				callArgs["label"] = args[i+1]
				i++
			}
		case "--path":
			if i+1 < len(args) {
				callArgs["path"] = args[i+1]
				i++
			}
		case "--parent-id":
			if i+1 < len(args) {
				callArgs["parent_id"] = args[i+1]
				i++
			}
		case "--module":
			if i+1 < len(args) {
				callArgs["module"] = args[i+1]
				i++
			}
		case "--hidden":
			callArgs["hidden"] = true
		case "--ghost":
			callArgs["ghost"] = true
		case "--sort-order":
			if i+1 < len(args) {
				n, err := strconv.Atoi(args[i+1])
				if err != nil {
					fatal("--sort-order must be a number: %v", err)
				}
				callArgs["sort_order"] = float64(n)
				i++
			}
		}
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "update_nav_item", callArgs)
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// runNavDelete deletes a navigation item via delete_nav_item MCP tool.
func runNavDelete(args []string) {
	if len(args) == 0 {
		fatal("nav delete requires an item ID")
	}
	id := args[0]

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "delete_nav_item", map[string]any{"id": id})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// navListTable renders a list_nav_items JSON result as an aligned table with
// columns ID, LABEL, PATH, PARENT, HIDDEN, GHOST, SORT. An empty list yields a
// short notice.
func navListTable(jsonText string) (string, error) {
	var resp struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(jsonText), &resp); err != nil {
		return "", fmt.Errorf("decode list_nav_items response: %w", err)
	}
	if len(resp.Items) == 0 {
		return "No nav items.\n", nil
	}
	rows := make([][]string, 0, len(resp.Items))
	for _, item := range resp.Items {
		id := asString(item["id"])
		label := asString(item["label"])
		path := asString(item["path"])
		parent := asString(item["parent_id"])
		hidden := boolCell(item["hidden"])
		ghost := boolCell(item["ghost"])
		sort := fmt.Sprintf("%v", item["sort_order"])
		rows = append(rows, []string{id, label, path, parent, hidden, ghost, sort})
	}
	return renderTable([]string{"ID", "LABEL", "PATH", "PARENT", "HIDDEN", "GHOST", "SORT"}, rows), nil
}

// boolCell returns "yes" or "no" for a boolean interface value.
func boolCell(v any) string {
	if b, ok := v.(bool); ok && b {
		return "yes"
	}
	return "no"
}
