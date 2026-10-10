package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// runRelationCommand dispatches `relation <verb> ...`: withdraw (through the
// withdraw_relation MCP tool) and list (through list_relations).
func runRelationCommand(args []string) {
	if len(args) > 0 && args[0] == "list" {
		runRelationList(args[1:])
		return
	}
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
		return nil, fmt.Errorf("relation requires a verb: withdraw or list")
	}
	if args[0] != "withdraw" {
		return nil, fmt.Errorf("unknown relation verb %q (want withdraw or list)", args[0])
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

// relationListArgs is a parsed `relation list`: list_relations' arguments plus
// the two output switches.
type relationListArgs struct {
	params map[string]any
	all    bool // page to total with the first page's as_of
	json   bool // print JSON instead of a table
}

// parseRelationListArgs reads `relation list [flags]`. A negative --limit or
// --offset, or a stray argument, is an error before any request.
func parseRelationListArgs(args []string) (relationListArgs, error) {
	fs := flag.NewFlagSet("relation list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	kind := fs.String("kind", "", "only this relation kind")
	sourceType := fs.String("source-type", "", "only edges whose source has this type")
	targetType := fs.String("target-type", "", "only edges whose target has this type")
	includeEnded := fs.Bool("include-ended", false, "also list edges that had ended by as_of")
	asOf := fs.String("as-of", "", "RFC3339 time the pages are cut at")
	limit := fs.Int("limit", 0, "page size (server default 50, max 500)")
	offset := fs.Int("offset", 0, "edges to skip")
	all := fs.Bool("all", false, "page through every edge")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return relationListArgs{}, err
	}
	if fs.NArg() > 0 {
		return relationListArgs{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if *limit < 0 || *offset < 0 {
		return relationListArgs{}, fmt.Errorf("--limit and --offset must not be negative")
	}
	params := map[string]any{}
	for k, v := range map[string]string{"kind": *kind, "source_type": *sourceType, "target_type": *targetType, "as_of": *asOf} {
		if v != "" {
			params[k] = v
		}
	}
	if *includeEnded {
		params["include_ended"] = true
	}
	if *limit > 0 {
		params["limit"] = *limit
	}
	if *offset > 0 {
		params["offset"] = *offset
	}
	return relationListArgs{params: params, all: *all, json: *asJSON}, nil
}

// relationListPage is one list_relations result.
type relationListPage struct {
	Edges  []map[string]any `json:"edges"`
	Total  int              `json:"total"`
	Count  int              `json:"count"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
	AsOf   string           `json:"as_of"`
}

// fetchRelationPages calls list_relations once, or with all set pages on with
// the first page's as_of until the offset reaches total (or a page comes back
// empty). The returned page holds every edge read.
func fetchRelationPages(call func(string, map[string]any) (string, error), params map[string]any, all bool) (relationListPage, error) {
	var out relationListPage
	for {
		text, err := call("list_relations", params)
		if err != nil {
			return relationListPage{}, err
		}
		var p relationListPage
		if err := json.Unmarshal([]byte(text), &p); err != nil {
			return relationListPage{}, fmt.Errorf("decode list_relations result: %w", err)
		}
		if out.AsOf == "" {
			out = p
		} else {
			out.Edges = append(out.Edges, p.Edges...)
			out.Total = p.Total
		}
		next := p.Offset + p.Count
		if !all || p.Count == 0 || next >= p.Total {
			out.Count = len(out.Edges)
			return out, nil
		}
		params["offset"] = next
		params["as_of"] = p.AsOf
	}
}

// printRelationTable writes the edges as a table.
func printRelationTable(w io.Writer, p relationListPage) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tKIND\tSOURCE\tTARGET\tCLASS\tENDED")
	for _, e := range p.Edges {
		ended := ""
		if end, ok := e["ended"].(map[string]any); ok {
			ended = fmt.Sprint(end["cause"])
		}
		fmt.Fprintf(tw, "%v\t%v\t%v/%v\t%v/%v\t%v\t%s\n",
			e["id"], e["relation_kind"], e["source_type"], e["source_id"], e["target_type"], e["target_id"], e["edge_class"], ended)
	}
	tw.Flush()
	fmt.Fprintf(w, "%d of %d edges, as of %s\n", len(p.Edges), p.Total, p.AsOf)
}

func runRelationList(args []string) {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			printRelationHelp()
			return
		}
	}
	la, err := parseRelationListArgs(args)
	if err != nil {
		fatal("%v", err)
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	call := func(tool string, params map[string]any) (string, error) { return mcpCall(cfg, tool, params) }
	page, err := fetchRelationPages(call, la.params, la.all)
	if err != nil {
		fatal("%v", err)
	}
	if la.json {
		raw, err := json.Marshal(page)
		if err != nil {
			fatal("%v", err)
		}
		if err := printJSON(raw); err != nil {
			fatal("%v", err)
		}
		return
	}
	printRelationTable(os.Stdout, page)
}

func printRelationHelp() {
	fmt.Print(strings.TrimLeft(`
smeldr-cli relation: list relations, or end one on purpose

Usage:
  smeldr-cli relation list [--kind K] [--source-type T] [--target-type T]
                           [--include-ended] [--as-of RFC3339]
                           [--limit N] [--offset N] [--all] [--json]
  smeldr-cli relation withdraw <id> --reason <text>

list pages through every relation on the instance (list_relations), ordered
by created_at then id. The first page fixes as_of; --all pages on with it to
the total, so one command prints the whole graph as it was at that moment.
Live edges only unless --include-ended. Source and target ids are the items'
raw ID, as the list tools return them. Requires Author role. Needs
smeldr.dev/mcp v1.58.0 or later on the server.

withdraw ends the live relation <id> (an edge id from get_relations or
relation list). Its row stays as history, ended now, and the end is recorded
with you as the actor and your reason (cause "withdrawn"). Asserting the same
relation again later starts a new row. A relation that has already ended is
refused. There is no delete for relations. Requires Author role (operation
archive). Needs smeldr.dev/mcp v1.51.0 or later on the server.
`, "\n"))
}
