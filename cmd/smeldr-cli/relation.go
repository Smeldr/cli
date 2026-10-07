package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

// runRelationCommand dispatches `relation <verb> ...`. Its one verb is
// withdraw, through the withdraw_relation MCP tool.
func runRelationCommand(args []string) {
	params, err := parseRelationArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		printRelationHelp()
		return
	}
	if err != nil {
		fatal("%v", err)
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "withdraw_relation", params)
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// parseRelationArgs reads `relation withdraw <id> --reason <text>` (the flag
// before or after the id) into withdraw_relation's arguments. A missing id or
// reason is an error before any request.
func parseRelationArgs(args []string) (map[string]any, error) {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			return nil, flag.ErrHelp
		}
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("relation requires a verb: withdraw")
	}
	if args[0] != "withdraw" {
		return nil, fmt.Errorf("unknown relation verb %q (want withdraw)", args[0])
	}
	fs := flag.NewFlagSet("relation withdraw", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	reason := fs.String("reason", "", "why the relation no longer holds (required)")
	if err := fs.Parse(args[1:]); err != nil {
		return nil, err
	}
	pos := fs.Args()
	if len(pos) > 1 {
		if err := fs.Parse(pos[1:]); err != nil {
			return nil, err
		}
		if fs.NArg() > 0 {
			return nil, fmt.Errorf("unexpected argument %q", fs.Arg(0))
		}
		pos = pos[:1]
	}
	if len(pos) < 1 || pos[0] == "" {
		return nil, fmt.Errorf("relation withdraw requires <id>")
	}
	if *reason == "" {
		return nil, fmt.Errorf("relation withdraw requires --reason")
	}
	return map[string]any{"id": pos[0], "reason": *reason}, nil
}

func printRelationHelp() {
	fmt.Print(`smeldr-cli relation: end a relation on purpose

Usage:
  smeldr-cli relation withdraw <id> --reason <text>

Ends the live relation <id> (an edge id from get_relations). Its row stays as
history, ended now, and the end is recorded with you as the actor and your
reason (cause "withdrawn"). Asserting the same relation again later starts a
new row. A relation that has already ended is refused. There is no delete for
relations. Requires Author role (operation archive). Needs smeldr.dev/mcp
v1.51.0 or later on the server.
`)
}
