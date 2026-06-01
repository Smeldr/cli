package main

import (
	"fmt"
	"os"
)

// runBlockCommand dispatches `block` subcommands (node / section / item),
// mirroring the T32 block MCP tools. args begins with the subcommand.
func runBlockCommand(args []string) {
	if len(args) == 0 {
		printBlockHelp(os.Stderr)
		os.Exit(1)
	}
	switch args[0] {
	case "-h", "--help", "help":
		printBlockHelp(os.Stdout)
	case "node":
		runNodeCommand(args[1:])
	case "section":
		runEdgeCommand("section", args[1:])
	case "item":
		runEdgeCommand("item", args[1:])
	default:
		fatal("unknown block subcommand %q — use: node section item", args[0])
	}
}

func printBlockHelp(w *os.File) {
	fmt.Fprint(w, `forge-cli block — block system (T32)

Subcommands:
  node    <verb> [args]   manage blocks (generic content nodes)
  section <verb> [args]   compose page sections
  item    <verb> [args]   compose collection items

Node verbs (Author role):
  create  --type <type_name> [--field K=V ...] [--fields <json>]
  update  <id> [--field K=V ...] [--fields <json>]
  get     <id>
  list    [--type <type_name>] [--status <s>] [--json]
  publish <id>
  archive <id>

Section / item verbs (Editor role):
  add     <parent_id> <child_id>
  reorder <parent_id> <child_id1,child_id2,...>
  remove  <parent_id> <child_id>

Block Fields keys are CASE-SENSITIVE — use PascalCase (Title, Body, Headline)
to match block templates. Blocks are addressed by ID (no slug).

Operations are sent to the MCP endpoint (FORGE_MCP_URL).
`)
}
