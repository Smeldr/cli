package main

import (
	"encoding/json"
	"errors"
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

// checkCall is a check subcommand's parsed arguments: the tool to call, its
// parameters, and whether to print the raw JSON.
type checkCall struct {
	tool   string
	params map[string]any
	raw    bool
}

// newCheckFlags is a subcommand's FlagSet: errors are returned, never an
// exit, so an unknown flag is refused with its name (T958).
func newCheckFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

// noPositionals refuses a stray argument to a subcommand that takes none.
func noPositionals(name string, pos []string) error {
	if len(pos) > 0 {
		return fmt.Errorf("%s: unexpected argument %q", name, pos[0])
	}
	return nil
}

// runCheckCall runs a parsed subcommand, or exits with err.
func runCheckCall(c checkCall, err error) string {
	if err != nil {
		fatal("%v", err)
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, c.tool, c.params)
	if err != nil {
		fatal("%v", err)
	}
	return text
}

func parseCheckChange(args []string) (checkCall, error) {
	fs := newCheckFlags("check change")
	repo := fs.String("repo", "", "repository, owner/name")
	number := fs.Int("number", 0, "pull request number")
	head := fs.String("head", "", "head commit, 40-character SHA")
	total := fs.Int("files-total", 0, "files the change touches (default: the paths given)")
	fromStdin := fs.Bool("stdin", false, "read paths one per line from stdin")
	raw := fs.Bool("json", false, "raw JSON")
	paths, err := parseInterspersed(fs, args)
	if err != nil {
		return checkCall{}, err
	}
	if *fromStdin {
		paths = append(paths, readLines(stdin)...)
	}
	if *repo == "" || *number <= 0 || *head == "" || len(paths) == 0 {
		return checkCall{}, errors.New("check change needs --repo, --number, --head and paths (arguments or --stdin)")
	}
	return checkCall{"check_change", map[string]any{"repository": *repo, "number": *number, "head_commit": *head, "paths": paths, "files_total": *total}, *raw}, nil
}

func runCheckChange(args []string) {
	c, err := parseCheckChange(args)
	printCheckView(runCheckCall(c, err), c.raw)
}

func parseCheckMerge(args []string) (checkCall, error) {
	fs := newCheckFlags("check merge")
	repo := fs.String("repo", "", "repository, owner/name")
	number := fs.Int("number", 0, "pull request number")
	head := fs.String("head", "", "the merged head commit")
	merge := fs.String("merge", "", "the commit on the default branch")
	raw := fs.Bool("json", false, "raw JSON")
	pos, err := parseInterspersed(fs, args)
	if err == nil {
		err = noPositionals("check merge", pos)
	}
	if err != nil {
		return checkCall{}, err
	}
	if *repo == "" || *number <= 0 || *head == "" || *merge == "" {
		return checkCall{}, errors.New("check merge needs --repo, --number, --head and --merge")
	}
	return checkCall{"record_merge", map[string]any{"repository": *repo, "number": *number, "head_commit": *head, "merge_commit": *merge}, *raw}, nil
}

func runCheckMerge(args []string) {
	c, err := parseCheckMerge(args)
	text := runCheckCall(c, err)
	if c.raw {
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

func parseCheckAnswer(args []string) (checkCall, error) {
	fs := newCheckFlags("check answer")
	approve := fs.Bool("approve", false, "approve the change for the Area")
	reject := fs.Bool("not-approved", false, "do not approve it (needs --reason)")
	reason := fs.String("reason", "", "why")
	raw := fs.Bool("json", false, "raw JSON")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return checkCall{}, err
	}
	if len(pos) != 2 || *approve == *reject {
		return checkCall{}, errors.New("check answer <check> <area> needs --approve or --not-approved (with --reason)")
	}
	return checkCall{"answer_change_check", map[string]any{"check": pos[0], "area": pos[1], "approve": *approve, "reason": *reason}, *raw}, nil
}

func runCheckAnswer(args []string) {
	c, err := parseCheckAnswer(args)
	printCheckView(runCheckCall(c, err), c.raw)
}

func parseCheckShow(args []string) (checkCall, error) {
	fs := newCheckFlags("check show")
	raw := fs.Bool("json", false, "raw JSON")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return checkCall{}, err
	}
	if len(pos) != 1 {
		return checkCall{}, errors.New("check show <check> [--json]")
	}
	return checkCall{"get_change_check", map[string]any{"check": pos[0]}, *raw}, nil
}

func runCheckShow(args []string) {
	c, err := parseCheckShow(args)
	printCheckView(runCheckCall(c, err), c.raw)
}

func parseCheckList(args []string) (checkCall, error) {
	fs := newCheckFlags("check list")
	repo := fs.String("repo", "", "repository, owner/name")
	number := fs.Int("number", 0, "pull request number")
	area := fs.String("area", "", "an Area")
	decision := fs.String("decision", "", "a Decision")
	status := fs.String("status", "", "one state")
	limit := fs.Int("limit", 0, "at most this many")
	raw := fs.Bool("json", false, "raw JSON")
	pos, err := parseInterspersed(fs, args)
	if err == nil {
		err = noPositionals("check list", pos)
	}
	if err != nil {
		return checkCall{}, err
	}
	return checkCall{"list_change_checks", map[string]any{"repository": *repo, "number": *number, "area": *area, "decision": *decision, "status": *status, "limit": *limit}, *raw}, nil
}

func runCheckList(args []string) {
	c, err := parseCheckList(args)
	text := runCheckCall(c, err)
	if c.raw {
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

Flags may come before or after the arguments; an unknown flag or a stray
argument is refused.

change and merge are for the host's adapter or an operator (check_change
capability). answer counts when you hold approve-change over that Area (or
item-change-approver on the Check). list needs a repository, an Area or a
Decision: Checks are read by filter.
`)
}
