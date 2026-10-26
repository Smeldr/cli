package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

// runDecisionCommand handles smeldr-cli decision sources (core T914, D148);
// any other subcommand is the generic content command for the decision path.
func runDecisionCommand(args []string) {
	if len(args) == 0 || args[0] != "sources" {
		runContentCommand("decision", args)
		return
	}
	runDecisionSourcesCommand(args[1:])
}

// decisionSource is one pinned source as the server returns it.
type decisionSource struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Path       string `json:"path"`
	Anchor     string `json:"anchor,omitempty"`
	Role       string `json:"role"`
	Hash       string `json:"hash,omitempty"`
}

// runDecisionSourcesCommand lists a Decision's pinned sources, or replaces
// them with --set <file.json> (- for stdin) or --clear, via get_decision and
// update_decision.
func runDecisionSourcesCommand(args []string) {
	if wantsHelp(args) {
		printDecisionSourcesHelp()
		return
	}
	fs := newCommandFlags("decision sources")
	fs.Usage = printDecisionSourcesHelp
	asJSON := fs.Bool("json", false, "raw JSON")
	clearFlag := fs.Bool("clear", false, "remove every source")
	set := fs.String("set", "", "a JSON file of sources, or - for stdin")
	pos := parseCommandFlags(fs, args, 1, "decision sources needs a Decision: address (D148), slug or ID")
	decision, setFrom, raw, clear := pos[0], *set, *asJSON, *clearFlag
	if setFlags(fs)["set"] && setFrom == "" {
		fatal("--set needs a JSON file, or - for stdin")
	}
	if decision == "" {
		fatal("decision sources needs a Decision: address (D148), slug or ID")
	}
	if clear && setFrom != "" {
		fatal("use --set or --clear, not both")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	if clear || setFrom != "" {
		sources := []decisionSource{}
		if setFrom != "" {
			var data []byte
			if setFrom == "-" {
				data, err = io.ReadAll(stdin)
			} else {
				data, err = os.ReadFile(setFrom)
			}
			if err != nil {
				fatal("read sources: %v", err)
			}
			if err := json.Unmarshal(data, &sources); err != nil {
				fatal("sources must be a JSON array of {repository, commit, path, anchor, role, hash}: %v", err)
			}
		}
		if _, err := mcpCall(cfg, "update_decision", map[string]any{"slug": decision, "sources": sources}); err != nil {
			fatal("%v", err)
		}
		fmt.Printf("%s: %d sources set\n", decision, len(sources))
		return
	}
	text, err := mcpCall(cfg, "get_decision", map[string]any{"slug": decision})
	if err != nil {
		fatal("%v", err)
	}
	var d struct {
		Sources []decisionSource `json:"sources"`
	}
	if err := json.Unmarshal([]byte(text), &d); err != nil {
		fatal("unexpected response: %v", err)
	}
	if raw {
		out, _ := json.MarshalIndent(d.Sources, "", "  ")
		fmt.Println(string(out))
		return
	}
	if len(d.Sources) == 0 {
		fmt.Printf("%s has no sources\n", decision)
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ROLE\tREPOSITORY\tCOMMIT\tPATH\tANCHOR")
	for _, s := range d.Sources {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Role, s.Repository, s.Commit[:min(12, len(s.Commit))], s.Path, s.Anchor)
	}
	w.Flush()
}

func printDecisionSourcesHelp() {
	fmt.Print(`smeldr-cli decision sources: a Decision's pinned external sources (D148)

Usage:
  smeldr-cli decision sources <decision> [--json]
  smeldr-cli decision sources <decision> --set <file.json|->
  smeldr-cli decision sources <decision> --clear

Each source is {repository: owner/name, commit: a full 40-character SHA (never
a branch), path: one file, anchor: optional, role: source|evidence|implementation,
hash: optional sha256:<64 hex>}; at most 20. --set replaces the whole list.
Sources are set while the Decision is proposed: a ratified Decision is locked,
so to re-pin it, propose a Decision that supersedes it. Needs smeldr.dev/core
with Decision sources (T914) on the server.
`)
}
