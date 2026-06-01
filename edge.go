package main

import (
	"fmt"
	"os"
	"strings"
)

// runEdgeCommand dispatches the composition verbs for either page sections
// (kind="section") or collection items (kind="item"). args begins with the verb.
func runEdgeCommand(kind string, args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: forge-cli block %s <add|reorder|remove> [args]\n", kind)
		os.Exit(1)
	}
	switch args[0] {
	case "-h", "--help", "help":
		printBlockHelp(os.Stdout)
	case "add":
		runEdgeAddRemove(kind, "add", args[1:])
	case "remove":
		runEdgeAddRemove(kind, "remove", args[1:])
	case "reorder":
		runEdgeReorder(kind, args[1:])
	default:
		fatal("unknown %s verb %q — use: add reorder remove", kind, args[0])
	}
}

// runEdgeAddRemove runs add_{kind} / remove_{kind} with a parent and child ID.
func runEdgeAddRemove(kind, verb string, args []string) {
	if len(args) < 2 {
		fatal("block %s %s requires <parent_id> <child_id>", kind, verb)
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, verb+"_"+kind, map[string]any{
		"parent_id": args[0],
		"child_id":  args[1],
	})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// runEdgeReorder runs reorder_{kind}s with a parent ID and a comma-separated
// ordered list of child IDs.
func runEdgeReorder(kind string, args []string) {
	if len(args) < 2 {
		fatal("block %s reorder requires <parent_id> <child_id1,child_id2,...>", kind)
	}
	ids := splitCSV(args[1])
	if len(ids) == 0 {
		fatal("block %s reorder requires a non-empty comma-separated child id list", kind)
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "reorder_"+kind+"s", map[string]any{
		"parent_id":         args[0],
		"ordered_child_ids": ids,
	})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// splitCSV splits a comma-separated list, trimming spaces and dropping empties.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
