package main

import (
	"fmt"
	"os"
	"strings"
)

// runTransitionCommand moves an item — dynamic content or a compiled type
// (e.g. "Decision", "Task", "Signal") — to a new state via the
// transition_item MCP tool. args is [type_name, slug, --to <state>,
// --reason <text>].
func runTransitionCommand(args []string) {
	if wantsHelp(args) {
		printTransitionHelp()
		return
	}

	args, rev, err := takeRev(args)
	if err != nil {
		fatal("%v", err)
		return
	}
	fs := newCommandFlags("transition")
	fs.Usage = printTransitionHelp
	to := fs.String("to", "", "the state to move to")
	why := fs.String("reason", "", "why")
	pos := parseCommandFlags(fs, args, 2, "transition requires <type_name> <slug> --to <state>")
	typeName, slug, toState, reason := pos[0], pos[1], *to, *why
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
	// A governance type names the rev the person read (core T1010).
	getTool := revGetTool(typeName)
	weighty := toState == "ratified" || toState == "superseded"
	if getTool != "" {
		if rev == nil {
			n := readAndConfirm(cfg, "transition "+typeName, "Move "+typeName+" to "+toState, getTool, map[string]any{"slug": slug}, weighty)
			rev = &n
		}
		callArgs["rev"] = *rev
	} else if rev != nil {
		callArgs["rev"] = *rev
	}
	var onConflict func()
	if getTool != "" && weighty {
		// Show what changed before the person decides again.
		onConflict = func() {
			if now, err := mcpCall(cfg, getTool, map[string]any{"slug": slug}); err == nil {
				n, _ := revOf(now)
				fmt.Fprintf(os.Stderr, "It is now:\n  %s\n", describeRecord(now, n, true))
			}
		}
	}
	text := writeCall(cfg, "transition_item", callArgs, onConflict)
	// core D189: the warning (printed by writeCall, with the server's date)
	// gets the way instead.
	if strings.Contains(text, "decision.supersede_without_replacement") {
		fmt.Fprintln(os.Stderr, "use smeldr-cli decision supersede to record the replacement, or decision retire")
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func printTransitionHelp() {
	fmt.Print(`smeldr-cli transition — move an item to a new state via its registered StateFlow

Usage:
  smeldr-cli transition <type_name> <slug> --to <state> [--reason <text>] [--rev <n>]

type_name is a registered content type name — dynamic (snake_case, e.g.
"posts") or compiled (e.g. "Decision", "Task", "Signal"). The transition is
validated against the type's registered flow; the target instance's own
role gate (RequiredRole/RequiredOperation) applies. --reason is required
only if the target transition itself requires one.

Example — ratifying a Decision:
  smeldr-cli transition Decision <slug> --to ratified

--rev is the rev of the record you read; it is needed for these types: `)
	fmt.Println(revTypes() + ".")
	fmt.Print(`Without --rev, on a terminal, cli shows the record and asks before it
writes; without a terminal it refuses.
`)
}
