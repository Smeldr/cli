package main

import (
	"encoding/json"
	"fmt"
)

// runStandingCommand prints one item's standing (D100) via the
// get_item_standing MCP tool. args is [type_name, slug, --json].
func runStandingCommand(args []string) {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		printStandingHelp()
		return
	}

	var typeName, slug string
	raw := false
	positional := 0
	for _, a := range args {
		if a == "--json" {
			raw = true
			continue
		}
		switch positional {
		case 0:
			typeName = a
		case 1:
			slug = a
		}
		positional++
	}
	if typeName == "" || slug == "" {
		fatal("standing requires <type_name> <slug>")
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "get_item_standing", map[string]any{
		"type_name": typeName,
		"slug":      slug,
	})
	if err != nil {
		fatal("%v", err)
	}
	if raw {
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
		return
	}
	var res struct {
		Standing *string `json:"standing"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		fatal("unexpected response: %v", err)
	}
	if res.Standing == nil {
		fmt.Println("no standing for this type")
		return
	}
	fmt.Println(*res.Standing)
}

func printStandingHelp() {
	fmt.Print(`smeldr-cli standing: show whether an item's claim is in force (D100)

Usage:
  smeldr-cli standing <type_name> <slug> [--json]

Prints holds, ceased or none (an item of a type that has standing but no
stored row is none), or "no standing for this type" when the type has no
standing at all (in rare cases the server could not read the type's flow, which
is logged on the server, and the output is the same). type_name is dynamic (snake_case) or compiled (e.g.
"Decision", "Amendment"). --json prints the tool's whole result instead.
Requires Editor role. Needs smeldr.dev/mcp v1.46.0 or later on the server.

Note: "<type> get" and "<type> list" read the REST routes and do not show
standing; use this command or the MCP get tools for it.

Example:
  smeldr-cli standing Decision <slug>
`)
}
