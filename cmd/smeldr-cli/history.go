package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
)

// runHistoryCommand prints one item's history (D101) via the get_item_provenance
// MCP tool. args is [type_name, slug, --limit n, --offset n, --view v], flags
// before or after the positional arguments.
func runHistoryCommand(args []string) {
	params, err := parseHistoryArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		printHistoryHelp()
		return
	}
	if err != nil {
		fatal("%v", err)
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "get_item_provenance", params)
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// parseHistoryArgs reads `history <type_name> <slug> [--limit n] [--offset n]
// [--view members|gated]` into the tool's arguments. Only what was given is sent,
// so the server's own defaults apply. A bad value is an error before any request.
func parseHistoryArgs(args []string) (map[string]any, error) {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			return nil, flag.ErrHelp
		}
	}
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	limit := fs.Int("limit", -1, "entries per page (default 50, at most 500)")
	offset := fs.Int("offset", -1, "entries to skip, newest first")
	view := fs.String("view", "", "members (default) or gated")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	pos := fs.Args()
	if len(pos) > 2 {
		// Flags after the positional arguments: parse the rest again.
		if err := fs.Parse(pos[2:]); err != nil {
			return nil, err
		}
		if fs.NArg() > 0 {
			return nil, fmt.Errorf("unexpected argument %q", fs.Arg(0))
		}
		pos = pos[:2]
	}
	if len(pos) < 2 || pos[0] == "" || pos[1] == "" {
		return nil, fmt.Errorf("history requires <type_name> <slug>")
	}
	params := map[string]any{"type_name": pos[0], "slug": pos[1]}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if set["limit"] {
		if *limit < 1 {
			return nil, fmt.Errorf("--limit must be a positive integer, got %s", strconv.Itoa(*limit))
		}
		params["limit"] = *limit
	}
	if set["offset"] {
		if *offset < 0 {
			return nil, fmt.Errorf("--offset must not be negative, got %s", strconv.Itoa(*offset))
		}
		params["offset"] = *offset
	}
	if set["view"] {
		if *view != "members" && *view != "gated" {
			return nil, fmt.Errorf(`--view must be "members" or "gated", got %q`, *view)
		}
		params["view"] = *view
	}
	return params, nil
}

func printHistoryHelp() {
	fmt.Print(`smeldr-cli history: show one item's history, newest first (D101)

Usage:
  smeldr-cli history <type_name> <slug> [--limit n] [--offset n] [--view members|gated]

Prints the tool's result as JSON: every recorded change with its time (UTC,
second resolution), verb (create, update, transition, standing-began,
standing-ended), from and to state, and whether the act was gated. By default
(--view members) every entry carries its actor: actor_kind (job, agent, human
or unclassified), actor_id, surface and reason. --view gated narrows the answer
to the rule for a wider audience: the actor appears only on a gated transition.
You can only narrow your view, never widen it. Rows written before core v1.121.0
say human for an actor with no classification and mean unclassified, never a
verified person. --limit defaults to 50 (at most 500); total is the whole
history. Relation events are not part of this read. type_name is dynamic
(snake_case) or compiled (e.g. "Decision", "Task").
Requires Editor role. Needs smeldr.dev/mcp v1.49.0 or later on the server, with
provenance enabled (an error says so when it is not).
`)
}
