package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// stdin is where --stdin reads paths from; tests replace it.
var stdin io.Reader = os.Stdin

// pathArgs reads the --repo, --stdin and --json flags and the paths of a
// coverage command (core T886): paths come as arguments, or one per line on
// standard input with --stdin.
func pathArgs(name string, args []string, extra func(flag string) bool) (repo string, paths []string, raw bool) {
	fromStdin := false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--repo":
			if i+1 >= len(args) {
				fatal("--repo needs owner/name")
			}
			repo = args[i+1]
			i++
		case a == "--stdin":
			fromStdin = true
		case a == "--json":
			raw = true
		case extra != nil && extra(a):
		case strings.HasPrefix(a, "--"):
			fatal("%s: unknown flag %s", name, a)
		default:
			paths = append(paths, a)
		}
	}
	if repo == "" {
		fatal("%s needs --repo owner/name", name)
	}
	if fromStdin {
		sc := bufio.NewScanner(stdin)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			if p := strings.TrimSpace(sc.Text()); p != "" {
				paths = append(paths, p)
			}
		}
		if err := sc.Err(); err != nil {
			fatal("read paths: %v", err)
		}
	}
	if len(paths) == 0 {
		fatal("%s needs paths: as arguments, or one per line with --stdin", name)
	}
	return repo, paths, raw
}

// runDecisionsForPathsCommand asks which Decisions apply to repository paths
// via the decisions_for_paths MCP tool (core T886).
func runDecisionsForPathsCommand(args []string) {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		printDecisionsForPathsHelp()
		return
	}
	includeOrg := true
	repo, paths, raw := pathArgs("decisions-for-paths", args, func(flag string) bool {
		if flag == "--no-organization" {
			includeOrg = false
			return true
		}
		return false
	})
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "decisions_for_paths", map[string]any{"repository": repo, "paths": paths, "include_organization": includeOrg})
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
		Areas []struct {
			Address      string   `json:"address"`
			Name         string   `json:"name"`
			ApprovalMode string   `json:"approval_mode"`
			Paths        []string `json:"paths"`
		} `json:"areas"`
		Decisions []struct {
			Address      string `json:"address"`
			ID           string `json:"id"`
			Title        string `json:"title"`
			Place        string `json:"place"`
			PlaceAddress string `json:"place_address"`
		} `json:"decisions"`
		Uncovered []string `json:"uncovered"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		fatal("unexpected response: %v", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if len(res.Areas) == 0 {
		fmt.Fprintln(w, "no Area covers these paths")
	} else {
		fmt.Fprintln(w, "AREA\tNAME\tAPPROVAL\tPATHS")
		for _, a := range res.Areas {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", a.Address, a.Name, a.ApprovalMode, len(a.Paths))
		}
	}
	if len(res.Decisions) > 0 {
		fmt.Fprintln(w, "\nDECISION\tPLACE\tTITLE")
		for _, d := range res.Decisions {
			record := d.Address
			if record == "" {
				record = d.ID
			}
			fmt.Fprintf(w, "%s\t%s %s\t%s\n", record, d.Place, d.PlaceAddress, d.Title)
		}
	}
	w.Flush()
	fmt.Printf("%d Decisions in force; %d paths uncovered\n", len(res.Decisions), len(res.Uncovered))
}

func printDecisionsForPathsHelp() {
	fmt.Print(`smeldr-cli decisions-for-paths: which Decisions apply to these files

Usage:
  smeldr-cli decisions-for-paths --repo owner/name [--stdin] [--no-organization] [--json] [<path>...]

Gives the Areas whose coverage rules match the paths, the Decisions in force
placed in those Areas, their Domains and on the Organization
(--no-organization leaves the last out), and the paths no rule covers. Paths
come as arguments or one per line with --stdin; at most 3000 per call.
Records you may not read never appear. Requires Author role. Needs
smeldr.dev/mcp with core's coverage rules (the places release) on the server.

Examples:
  git diff --name-only main | smeldr-cli decisions-for-paths --repo acme/app --stdin
  smeldr-cli decisions-for-paths --repo acme/app internal/billing/invoice.go
`)
}

// runCoverageCommand runs `coverage check` via the check_coverage MCP tool
// (core T886).
func runCoverageCommand(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printCoverageHelp()
		return
	}
	if args[0] != "check" {
		fatal("coverage: unknown subcommand %q (try: coverage check)", args[0])
	}
	repo, paths, raw := pathArgs("coverage check", args[1:], nil)
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "check_coverage", map[string]any{"repository": repo, "paths": paths})
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
		Paths int `json:"paths"`
		Rules []struct {
			Rule    string `json:"rule"`
			Area    string `json:"area"`
			Pattern string `json:"pattern"`
			Matches int    `json:"matches"`
		} `json:"rules"`
		MatchingNothing []string `json:"matching_nothing"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		fatal("unexpected response: %v", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RULE\tAREA\tMATCHES\tPATTERN")
	for _, r := range res.Rules {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", r.Rule, r.Area, r.Matches, r.Pattern)
	}
	w.Flush()
	fmt.Printf("%d paths, %d rules, %d matching nothing\n", res.Paths, len(res.Rules), len(res.MatchingNothing))
}

func printCoverageHelp() {
	fmt.Print(`smeldr-cli coverage check: count what each coverage rule matches

Usage:
  smeldr-cli coverage check --repo owner/name [--stdin] [--json] [<path>...]

Give the repository's full file list (one per line with --stdin, at most
100000). Each coverage rule of the repository gets its match count stored; a
rule that matches nothing opens a coverage-matches-nothing condition, which
resolves when it matches again or is changed. Requires Editor role. Needs
smeldr.dev/mcp with core's coverage rules (the places release) on the server.

Example:
  git ls-files | smeldr-cli coverage check --repo acme/app --stdin
`)
}
