package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"
)

// runSearchCommand searches every record type the caller may read via the
// search MCP tool (T885). args is [text] [--type t]... [--state s]...
// [--place p] [--limit n] [--offset n] [--json].
func runSearchCommand(args []string) {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		printSearchHelp()
		return
	}
	call := map[string]any{}
	var types, states []string
	raw := false
	value := func(i int, flag string) string {
		if i+1 >= len(args) {
			fatal("%s needs a value", flag)
		}
		return args[i+1]
	}
	number := func(i int, flag string) int {
		n, err := strconv.Atoi(value(i, flag))
		if err != nil || n < 0 {
			fatal("%s must be a number of 0 or more", flag)
		}
		return n
	}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--json":
			raw = true
		case "--type":
			types = append(types, value(i, a))
			i++
		case "--state":
			states = append(states, value(i, a))
			i++
		case "--place":
			call["place"] = value(i, a)
			i++
		case "--limit":
			call["limit"] = number(i, a)
			i++
		case "--offset":
			call["offset"] = number(i, a)
			i++
		default:
			if _, set := call["query"]; set {
				fatal("search takes one text; quote it if it has spaces")
			}
			call["query"] = a
		}
	}
	if len(types) > 0 {
		call["types"] = types
	}
	if len(states) > 0 {
		call["states"] = states
	}
	if len(call) == 0 {
		fatal("search needs a text or at least one of --type, --state, --place")
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "search", call)
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
		Hits []struct {
			Type    string `json:"type"`
			ID      string `json:"id"`
			Address string `json:"address"`
			Title   string `json:"title"`
			State   string `json:"state"`
			Snippet string `json:"snippet"`
		} `json:"hits"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		fatal("unexpected response: %v", err)
	}
	if len(res.Hits) == 0 {
		fmt.Printf("no hits (total %d)\n", res.Total)
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RECORD\tTYPE\tSTATE\tTITLE")
	for _, h := range res.Hits {
		record := h.Address
		if record == "" {
			record = h.ID
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", record, h.Type, h.State, h.Title)
		if h.Snippet != "" {
			fmt.Fprintf(w, "\t\t\t  %s\n", h.Snippet)
		}
	}
	w.Flush()
	fmt.Printf("%d of %d\n", len(res.Hits), res.Total)
}

func printSearchHelp() {
	fmt.Print(`smeldr-cli search: find records across every type you may read

Usage:
  smeldr-cli search [<text>] [--type <t>]... [--state <s>]... [--place <p>|none]
                    [--limit n] [--offset n] [--json]

Matches plain text, case-insensitively, in each type's searchable fields
(title, body, description, summary, message and the like); 2 to 200
characters. Records you may not read never appear and never count. The text
may be left out when a filter is set. --place none lists records without a
place (no Domain or Area), for example imported Decisions in the inbox; a
Domain or Area by address, slug or ID lists the records placed there. Newest
first, 50 per page (at most 500). --json prints the tool's whole result.
Requires Author role. Needs smeldr.dev/mcp v1.65.0 or later on the server.

Examples:
  smeldr-cli search "release plan"
  smeldr-cli search --type Decision --state proposed --place none
`)
}
