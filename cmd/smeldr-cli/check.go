package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"
)

// runCheckCommand is smeldr-cli check: the pull request check (core T918)
// over its MCP tools.
func runCheckCommand(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printCheckHelp()
		return
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "change":
		runCheckChange(rest)
	case "merge":
		runCheckMerge(rest)
	case "answer":
		runCheckAnswer(rest)
	case "show":
		runCheckShow(rest)
	case "list":
		runCheckList(rest)
	default:
		fatal("unknown check command %q: change, merge, answer, show or list", sub)
	}
}

// checkView is a Check as the tools return it.
type checkView struct {
	Covered bool `json:"covered"`
	Check   *struct {
		Address       string `json:"address"`
		Status        string `json:"Status"`
		Repository    string `json:"repository"`
		Number        int    `json:"number"`
		HeadCommit    string `json:"head_commit"`
		FilesTotal    int    `json:"files_total"`
		FilesChecked  int    `json:"files_checked"`
		CheckedPartly bool   `json:"checked_partly"`
		Setting       string `json:"setting"`
	} `json:"check"`
	Areas []struct {
		Address      string `json:"address"`
		Name         string `json:"name"`
		ApprovalMode string `json:"approval_mode"`
		Verdict      string `json:"verdict"`
	} `json:"areas"`
	Decisions []struct {
		Address string `json:"address"`
		Title   string `json:"title"`
	} `json:"decisions"`
	Uncovered []string `json:"uncovered"`
}

// printCheckView prints a Check, or the raw JSON with raw.
func printCheckView(text string, raw bool) {
	if raw {
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
		return
	}
	var v checkView
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		fatal("unexpected response: %v", err)
	}
	if v.Check == nil {
		fmt.Printf("no Area covers these paths: no Check (%d uncovered)\n", len(v.Uncovered))
		return
	}
	c := v.Check
	fmt.Printf("%s  %s  %s#%d at %.12s  %d/%d files", c.Address, c.Status, c.Repository, c.Number, c.HeadCommit, c.FilesChecked, c.FilesTotal)
	if c.CheckedPartly {
		fmt.Print(" (checked partly)")
	}
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if len(v.Areas) > 0 {
		fmt.Fprintln(w, "AREA\tNAME\tMODE\tVERDICT")
		for _, a := range v.Areas {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a.Address, a.Name, a.ApprovalMode, a.Verdict)
		}
	}
	if len(v.Decisions) > 0 {
		fmt.Fprintln(w, "\nDECISION\tTITLE")
		for _, d := range v.Decisions {
			fmt.Fprintf(w, "%s\t%s\n", d.Address, d.Title)
		}
	}
	w.Flush()
}

func runCheckChange(args []string) {
	fs := flag.NewFlagSet("check change", flag.ExitOnError)
	repo := fs.String("repo", "", "repository, owner/name")
	number := fs.Int("number", 0, "pull request number")
	head := fs.String("head", "", "head commit, 40-character SHA")
	total := fs.Int("files-total", 0, "files the change touches (default: the paths given)")
	fromStdin := fs.Bool("stdin", false, "read paths one per line from stdin")
	raw := fs.Bool("json", false, "raw JSON")
	_ = fs.Parse(args)
	paths := fs.Args()
	if *fromStdin {
		paths = append(paths, readLines(stdin)...)
	}
	if *repo == "" || *number <= 0 || *head == "" || len(paths) == 0 {
		fatal("check change needs --repo, --number, --head and paths (arguments or --stdin)")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "check_change", map[string]any{"repository": *repo, "number": *number, "head_commit": *head, "paths": paths, "files_total": *total})
	if err != nil {
		fatal("%v", err)
	}
	printCheckView(text, *raw)
}

func runCheckMerge(args []string) {
	fs := flag.NewFlagSet("check merge", flag.ExitOnError)
	repo := fs.String("repo", "", "repository, owner/name")
	number := fs.Int("number", 0, "pull request number")
	head := fs.String("head", "", "the merged head commit")
	merge := fs.String("merge", "", "the commit on the default branch")
	raw := fs.Bool("json", false, "raw JSON")
	_ = fs.Parse(args)
	if *repo == "" || *number <= 0 || *head == "" || *merge == "" {
		fatal("check merge needs --repo, --number, --head and --merge")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "record_merge", map[string]any{"repository": *repo, "number": *number, "head_commit": *head, "merge_commit": *merge})
	if err != nil {
		fatal("%v", err)
	}
	if *raw {
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
		return
	}
	var res struct {
		Covered      bool     `json:"covered"`
		HeadMismatch bool     `json:"head_mismatch"`
		Opened       []string `json:"opened"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		fatal("unexpected response: %v", err)
	}
	switch {
	case !res.Covered:
		fmt.Println("no Check for this change: nothing to audit")
	case len(res.Opened) == 0:
		fmt.Println("no audit opened")
	default:
		fmt.Printf("audit opened for %v\n", res.Opened)
	}
	if res.HeadMismatch {
		fmt.Println("note: the merged head is not the checked head")
	}
}

func runCheckAnswer(args []string) {
	fs := flag.NewFlagSet("check answer", flag.ExitOnError)
	approve := fs.Bool("approve", false, "approve the change for the Area")
	reject := fs.Bool("not-approved", false, "do not approve it (needs --reason)")
	reason := fs.String("reason", "", "why")
	raw := fs.Bool("json", false, "raw JSON")
	_ = fs.Parse(args)
	if fs.NArg() != 2 || *approve == *reject {
		fatal("check answer <check> <area> needs --approve or --not-approved (with --reason)")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "answer_change_check", map[string]any{"check": fs.Arg(0), "area": fs.Arg(1), "approve": *approve, "reason": *reason})
	if err != nil {
		fatal("%v", err)
	}
	printCheckView(text, *raw)
}

func runCheckShow(args []string) {
	raw := false
	var check string
	for _, a := range args {
		if a == "--json" {
			raw = true
		} else {
			check = a
		}
	}
	if check == "" {
		fatal("check show <check> [--json]")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "get_change_check", map[string]any{"check": check})
	if err != nil {
		fatal("%v", err)
	}
	printCheckView(text, raw)
}

func runCheckList(args []string) {
	fs := flag.NewFlagSet("check list", flag.ExitOnError)
	repo := fs.String("repo", "", "repository, owner/name")
	number := fs.Int("number", 0, "pull request number")
	area := fs.String("area", "", "an Area")
	decision := fs.String("decision", "", "a Decision")
	status := fs.String("status", "", "one state")
	limit := fs.Int("limit", 0, "at most this many")
	raw := fs.Bool("json", false, "raw JSON")
	_ = fs.Parse(args)
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "list_change_checks", map[string]any{"repository": *repo, "number": *number, "area": *area, "decision": *decision, "status": *status, "limit": *limit})
	if err != nil {
		fatal("%v", err)
	}
	if *raw {
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
		return
	}
	var res struct {
		Checks []struct {
			Address    string `json:"address"`
			Status     string `json:"Status"`
			Repository string `json:"repository"`
			Number     int    `json:"number"`
			HeadCommit string `json:"head_commit"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		fatal("unexpected response: %v", err)
	}
	if len(res.Checks) == 0 {
		fmt.Println("no Checks")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CHECK\tSTATUS\tCHANGE\tHEAD")
	for _, c := range res.Checks {
		fmt.Fprintf(w, "%s\t%s\t%s#%s\t%.12s\n", c.Address, c.Status, c.Repository, strconv.Itoa(c.Number), c.HeadCommit)
	}
	w.Flush()
}

func printCheckHelp() {
	fmt.Print(`smeldr-cli check: the pull request check (core T918)

Usage:
  smeldr-cli check change --repo owner/name --number N --head SHA [--files-total N] [--stdin] [--json] [<path>...]
  smeldr-cli check merge  --repo owner/name --number N --head SHA --merge SHA [--json]
  smeldr-cli check answer <check> <area> --approve | --not-approved --reason "why" [--json]
  smeldr-cli check show   <check> [--json]
  smeldr-cli check list   [--repo owner/name [--number N]] [--area A] [--decision D] [--status S] [--limit N] [--json]

change and merge are for the host's adapter or an operator (check_change
capability). answer counts when you hold approve-change over that Area (or
item-change-approver on the Check). list needs a repository, an Area or a
Decision: Checks are read by filter.
`)
}
