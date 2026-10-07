package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

// runReachabilityCommand prints what is reachable from one item in the relation
// graph via the get_reachability MCP tool. args is [type_name, id, flags...],
// flags before or after the positional arguments.
func runReachabilityCommand(args []string) {
	params, err := parseReachabilityArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		printReachabilityHelp()
		return
	}
	if err != nil {
		fatal("%v", err)
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "get_reachability", params)
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// parseReachabilityArgs reads `reachability <type_name> <id> [--kind k]
// [--direction d] [--depth n] [--max-items n] [--limit n] [--offset n]` into the
// tool's arguments. Only what was given is sent, so the server's defaults apply.
// A bad value is an error before any request.
func parseReachabilityArgs(args []string) (map[string]any, error) {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			return nil, flag.ErrHelp
		}
	}
	fs := flag.NewFlagSet("reachability", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	kind := fs.String("kind", "", "only walk this relation kind")
	direction := fs.String("direction", "", "incoming, outgoing or both (default both)")
	depth := fs.Int("depth", 0, "hops, 1 to 10 (default 1)")
	maxItems := fs.Int("max-items", 0, "cap on items the walk returns (default 500, at most 2000)")
	limit := fs.Int("limit", 0, "page size (default 100, at most 500)")
	offset := fs.Int("offset", 0, "items to skip")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	pos := fs.Args()
	if len(pos) > 2 {
		if err := fs.Parse(pos[2:]); err != nil {
			return nil, err
		}
		if fs.NArg() > 0 {
			return nil, fmt.Errorf("unexpected argument %q", fs.Arg(0))
		}
		pos = pos[:2]
	}
	if len(pos) < 2 || pos[0] == "" || pos[1] == "" {
		return nil, fmt.Errorf("reachability requires <type_name> <id>")
	}
	params := map[string]any{"type_name": pos[0], "id": pos[1]}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if set["kind"] {
		params["kind"] = *kind
	}
	if set["direction"] {
		if *direction != "incoming" && *direction != "outgoing" && *direction != "both" {
			return nil, fmt.Errorf(`--direction must be "incoming", "outgoing" or "both", got %q`, *direction)
		}
		params["direction"] = *direction
	}
	if set["depth"] {
		if *depth < 1 || *depth > 10 {
			return nil, fmt.Errorf("--depth must be between 1 and 10, got %d", *depth)
		}
		params["depth"] = *depth
	}
	if set["max-items"] {
		if *maxItems < 1 || *maxItems > 2000 {
			return nil, fmt.Errorf("--max-items must be between 1 and 2000, got %d", *maxItems)
		}
		params["max_items"] = *maxItems
	}
	if set["limit"] {
		if *limit < 1 || *limit > 500 {
			return nil, fmt.Errorf("--limit must be between 1 and 500, got %d", *limit)
		}
		params["limit"] = *limit
	}
	if set["offset"] {
		if *offset < 0 {
			return nil, fmt.Errorf("--offset must not be negative, got %d", *offset)
		}
		params["offset"] = *offset
	}
	return params, nil
}

func printReachabilityHelp() {
	fmt.Print(`smeldr-cli reachability: what is reachable from one item in the relation graph

Usage:
  smeldr-cli reachability <type_name> <id> [--kind k] [--direction incoming|outgoing|both]
                          [--depth n] [--max-items n] [--limit n] [--offset n]

Prints the tool's result as JSON: the items found at each hop distance (up to
10), each with its depth, type, id and the edge_class and confidence of the edge
that reached it. Live edges only: a relation that has ended is not walked.
direction is the walk's own vocabulary (incoming, outgoing, both; default both),
not the source and target of relation queries. The walk is bounded: --max-items
(default 500, at most 2000) caps how many items it returns. When the cap stops
the walk, "cut" says where: {depth, dropped} means the cap landed in ring depth
and dropped items found there were not returned; deeper rings were not walked
and are absent (never shown as empty). ring_sizes gives the count per returned
ring and total the number of items the walk returned; --limit (default 100, at
most 500) and --offset page the items. Graph structure only, no item content.
Requires Author role. Needs smeldr.dev/mcp v1.50.0 or later on the server.
`)
}
