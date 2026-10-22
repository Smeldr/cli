package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
)

// runApproversCommand lists who approves a place via the approvers_of MCP
// tool (core T911, D147).
func runApproversCommand(args []string) {
	var place string
	raw := false
	for _, a := range args {
		switch a {
		case "-h", "--help", "help":
			printApproversHelp()
			return
		case "--json":
			raw = true
		default:
			if place != "" {
				fatal("approvers takes one place, got %q and %q", place, a)
			}
			place = a
		}
	}
	if place == "" {
		fatal("approvers needs a place: an Organization, Domain or Area by address, slug or ID (for example DOM4)")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "approvers_of", map[string]any{"place": place})
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
		Approvers []struct {
			Name          string `json:"name"`
			Role          string `json:"role"`
			Class         string `json:"class"`
			AnchorAddress string `json:"anchor_address"`
		} `json:"approvers"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		fatal("unexpected response: %v", err)
	}
	if len(res.Approvers) == 0 {
		fmt.Printf("nobody approves %s\n", place)
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tROLE\tCLASS\tANCHOR")
	for _, a := range res.Approvers {
		anchor := a.AnchorAddress
		if anchor == "" {
			anchor = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a.Name, a.Role, a.Class, anchor)
	}
	w.Flush()
}

func printApproversHelp() {
	fmt.Print(`smeldr-cli approvers: who approves a place

Usage:
  smeldr-cli approvers <place> [--json]

<place> is an Organization, Domain or Area by address, slug or ID (DOM4, ARE3).
Lists every approver, the domain-approvers of the place's Domain and, for an
Area, its area-approvers: token name, role, class and the anchor's address
("-" when you may not read it). Only for a place you can read. Author role;
needs smeldr.dev/mcp with approvers_of (core T911) on the server.
`)
}
