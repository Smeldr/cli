package main

import "fmt"

// runTransitionCommand moves an item — dynamic content or a compiled type
// (e.g. "Decision", "Task", "Signal") — to a new state via the
// transition_item MCP tool. args is [type_name, slug, --to <state>,
// --reason <text>].
func runTransitionCommand(args []string) {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		printTransitionHelp()
		return
	}

	var typeName, slug, toState, reason string
	positional := 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--to":
			if i+1 < len(args) {
				toState = args[i+1]
				i++
			}
		case "--reason":
			if i+1 < len(args) {
				reason = args[i+1]
				i++
			}
		default:
			switch positional {
			case 0:
				typeName = args[i]
			case 1:
				slug = args[i]
			}
			positional++
		}
	}

	if typeName == "" || slug == "" {
		fatal("transition requires <type_name> <slug> --to <state>")
	}
	if toState == "" {
		fatal("transition requires --to <state>")
	}

	callArgs := map[string]any{
		"type_name": typeName,
		"slug":      slug,
		"to_state":  toState,
	}
	if reason != "" {
		callArgs["reason"] = reason
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "transition_item", callArgs)
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func printTransitionHelp() {
	fmt.Print(`smeldr-cli transition — move an item to a new state via its registered StateFlow

Usage:
  smeldr-cli transition <type_name> <slug> --to <state> [--reason <text>]

type_name is a registered content type name — dynamic (snake_case, e.g.
"posts") or compiled (e.g. "Decision", "Task", "Signal"). The transition is
validated against the type's registered flow; the target instance's own
role gate (RequiredRole/RequiredOperation) applies. --reason is required
only if the target transition itself requires one.

Example — ratifying a Decision:
  smeldr-cli transition Decision <slug> --to ratified
`)
}
