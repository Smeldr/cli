package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// restCount is one direction of a rest count as the tools return it.
type restCount struct {
	Open    int  `json:"open"`
	Closed  int  `json:"closed"`
	AtLeast bool `json:"at_least"`
}

// String writes the count, "at least" when the walk stopped at its cap.
func (c restCount) String() string {
	s := fmt.Sprintf("%d open, %d closed", c.Open, c.Closed)
	if c.AtLeast {
		s = "at least " + s
	}
	return s
}

// runRestsCommand is smeldr-cli rests: what rests on an item and what it
// rests on (core T919, D150), live with get_rest_counts or as stored with
// list_rest_counts.
func runRestsCommand(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printRestsHelp()
		return
	}
	fs := newCommandFlags("rests")
	fs.Usage = printRestsHelp
	storedF := fs.Bool("stored", false, "the last sweep's stored counts")
	asJSON := fs.Bool("json", false, "raw JSON")
	// list_rest_counts takes at most 200 items.
	rest := parseCommandFlagsAtMost(fs, args, 200)
	stored, raw := *storedF, *asJSON
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	if stored {
		runRestsStored(cfg, rest, raw)
		return
	}
	if len(rest) != 2 {
		fatal("rests <type> <item> [--json], or rests --stored <type>:<item>...")
	}
	text, err := mcpCall(cfg, "get_rest_counts", map[string]any{"type": rest[0], "id": rest[1]})
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
		RestsOnThis restCount `json:"rests_on_this"`
		ThisRestsOn restCount `json:"this_rests_on"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		fatal("unexpected response: %v", err)
	}
	fmt.Printf("rests on this: %s\nthis rests on: %s\n", res.RestsOnThis, res.ThisRestsOn)
}

// runRestsStored prints the stored counts of items given as <type>:<item>.
func runRestsStored(cfg Config, refs []string, raw bool) {
	if len(refs) == 0 {
		fatal("rests --stored needs items as <type>:<item>, for example Decision:D12")
	}
	items := make([]any, 0, len(refs))
	for _, r := range refs {
		typeName, ident, ok := strings.Cut(r, ":")
		if !ok || typeName == "" || ident == "" {
			fatal("%q is not <type>:<item>", r)
		}
		items = append(items, map[string]any{"type": typeName, "id": ident})
	}
	text, err := mcpCall(cfg, "list_rest_counts", map[string]any{"items": items})
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
		Items []struct {
			Type        string    `json:"type"`
			ID          string    `json:"id"`
			RestsOnThis restCount `json:"rests_on_this"`
			ThisRestsOn restCount `json:"this_rests_on"`
			CountedAt   string    `json:"counted_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		fatal("unexpected response: %v", err)
	}
	if len(res.Items) == 0 {
		fmt.Println("no item you can read")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ITEM\tRESTS ON THIS\tTHIS RESTS ON\tCOUNTED")
	for _, it := range res.Items {
		counted := it.CountedAt
		if counted == "" {
			counted = "never"
		}
		fmt.Fprintf(w, "%s %s\t%s\t%s\t%s\n", it.Type, it.ID, it.RestsOnThis, it.ThisRestsOn, counted)
	}
	w.Flush()
}

func printRestsHelp() {
	fmt.Print(`smeldr-cli rests: what rests on an item, and what it rests on (core T919)

Usage:
  smeldr-cli rests <type> <item> [--json]
  smeldr-cli rests --stored [--json] <type>:<item>...

The live form counts the items reached over grounded_in, addresses and amends,
transitively, each once, open and closed, only those you may read; "at least"
when the walk stopped at 1,000. --stored reads what the last sweep stored for
up to 200 items, summed over the places you read: it may be lower than the
live count, never higher. Items are named by address, slug or ID.
`)
}

// runPremiseCommand is smeldr-cli premise: the premise-flagged condition
// (core T919, D149).
func runPremiseCommand(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printPremiseHelp()
		return
	}
	if args[0] != "confirm" {
		fatal("unknown premise command %q: confirm", args[0])
	}
	fs := newCommandFlags("premise confirm")
	fs.Usage = printPremiseHelp
	reasonF := fs.String("reason", "", "why the Decision still holds")
	rest := parseCommandFlagsAtMost(fs, args[1:], 2)
	reason := *reasonF
	if len(rest) < 1 || len(rest) > 2 || strings.TrimSpace(reason) == "" {
		fatal("premise confirm <condition> --reason \"why\", or premise confirm <decision> <premise> --reason \"why\"")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	call := map[string]any{"condition": rest[0], "reason": reason}
	if len(rest) == 2 {
		call = map[string]any{"decision": rest[0], "premise": rest[1], "reason": reason}
	}
	if _, err := mcpCall(cfg, "confirm_premise", call); err != nil {
		fatal("%v", err)
	}
	if len(rest) == 1 {
		fmt.Printf("confirmed: the condition %s is resolved; its Decision still holds\n", rest[0])
		return
	}
	fmt.Printf("confirmed: %s still holds although %s is not in force\n", rest[0], rest[1])
}

func printPremiseHelp() {
	fmt.Print(`smeldr-cli premise: a Decision resting on one that is not in force (core D149)

Usage:
  smeldr-cli premise confirm <condition> --reason "why"
  smeldr-cli premise confirm <decision> <premise> --reason "why"

Resolves the premise-flagged condition: the Decision still holds. Name the
condition (its Signal address) when you may not read the premise.
It stays closed until the premise's state changes. Needs ratify over the
Decision's place; agents never confirm.
`)
}
