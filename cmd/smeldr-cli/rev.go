package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// The rev a governance write names is the version of the record the person
// read (core T1010). With --rev it is sent as given. Without it, on a
// terminal, cli shows the record and asks before it writes, so what the person
// confirms is the version they saw; without a terminal it refuses. A silent
// read right before the write would always pass, and is never done.

// isTerminal reports whether stdin is an interactive terminal
// (stdinIsTerminal, per platform). A variable, so tests can set it.
var isTerminal = stdinIsTerminal

// takeRev removes --rev <n> or --rev=<n> from args, wherever it stands, and
// returns the rest and the rev (nil when not given). A rev that is not a
// non-negative integer is an error before any request.
func takeRev(args []string) ([]string, *int, error) {
	var rest []string
	var rev *int
	for i := 0; i < len(args); i++ {
		a := args[i]
		var val string
		switch {
		case a == "--rev" || a == "-rev":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--rev needs a number: the rev of the record you read")
			}
			val = args[i+1]
			i++
		case strings.HasPrefix(a, "--rev=") || strings.HasPrefix(a, "-rev="):
			val = a[strings.Index(a, "=")+1:]
		default:
			rest = append(rest, a)
			continue
		}
		n, err := strconv.Atoi(val)
		if err != nil || n < 0 {
			return nil, nil, fmt.Errorf("--rev must be a whole number (the rev of the record you read), got %q", val)
		}
		rev = &n
	}
	return rest, rev, nil
}

// revOf reads the rev of a record from a get result: "rev" or "Rev".
func revOf(text string) (int, bool) {
	var m map[string]any
	if json.Unmarshal([]byte(text), &m) != nil {
		return 0, false
	}
	for _, k := range []string{"rev", "Rev"} {
		if v, ok := m[k].(float64); ok {
			return int(v), true
		}
	}
	return 0, false
}

// fieldOf reads a string field by name in any case.
func fieldOf(m map[string]any, name string) string {
	for k, v := range m {
		if strings.EqualFold(k, name) {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// describeRecord is what a person is shown before a write: address, title,
// status and rev, and with withBody the body.
func describeRecord(text string, rev int, withBody bool) string {
	var m map[string]any
	if json.Unmarshal([]byte(text), &m) != nil {
		return fmt.Sprintf("rev %d", rev)
	}
	var b strings.Builder
	name := fieldOf(m, "address")
	if name == "" {
		name = fieldOf(m, "slug")
	}
	title := fieldOf(m, "title")
	if title == "" {
		title = fieldOf(m, "name")
	}
	fmt.Fprintf(&b, "%s", name)
	if title != "" {
		fmt.Fprintf(&b, " %q", title)
	}
	if status := fieldOf(m, "status"); status != "" {
		fmt.Fprintf(&b, ", %s", status)
	}
	fmt.Fprintf(&b, ", rev %d", rev)
	if body := fieldOf(m, "body"); withBody && body != "" {
		fmt.Fprintf(&b, "\n\n%s\n", body)
	}
	return b.String()
}

// needTerminal refuses a write without --rev when no person can confirm it.
func needTerminal(write, where string) {
	if !isTerminal() {
		fatal("%s needs --rev <n>, the rev of the record you read (%s): without a terminal cli does not read and write on your behalf%s", write, where, terminalHint)
	}
}

// confirmRev shows what the write acts on and asks. Anything but y or yes
// exits 1 with nothing written.
func confirmRev(action, shown string, rev int) {
	fmt.Fprintf(os.Stderr, "%s:\n  %s\nProceed with rev %d? [y/N] ", action, shown, rev)
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return
	}
	fatal("not confirmed: nothing written")
}

// readAndConfirm reads the record with getTool and getArgs, shows it and asks,
// and returns the rev the person confirmed. Used when --rev is not given.
func readAndConfirm(cfg Config, write, action, getTool string, getArgs map[string]any, withBody bool) int {
	needTerminal(write, getTool)
	text, err := mcpCall(cfg, getTool, getArgs)
	if err != nil {
		fatal("%v", err)
	}
	rev, ok := revOf(text)
	if !ok {
		fatal("%s returned no rev: the server predates core T1010; pass --rev", getTool)
	}
	confirmRev(action, describeRecord(text, rev, withBody), rev)
	return rev
}

// isRevConflict reports whether err is core's rev_conflict (MCP -32002).
func isRevConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "MCP error -32002")
}

// writeCall makes a governance write: it prints the result's warnings to
// stderr and, on a rev_conflict, core's message, and exits 1 (never a retry).
// onConflict, when set, runs before the exit, to show what changed.
func writeCall(cfg Config, tool string, args map[string]any, onConflict func()) string {
	text, err := mcpCall(cfg, tool, args)
	if isRevConflict(err) {
		fmt.Fprintf(os.Stderr, "smeldr-cli: %v\n", err)
		if onConflict != nil {
			onConflict()
		}
		fatal("nothing written: read the record again and apply your change only if it still holds")
		return ""
	}
	if err != nil {
		fatal("%v", err)
		return ""
	}
	printWarnings(text)
	return text
}

// printWarnings prints a result's warnings (rev_missing and others) to stderr.
func printWarnings(text string) {
	var res struct {
		Warnings []struct {
			Code    string `json:"code"`
			Field   string `json:"field"`
			Message string `json:"message"`
		} `json:"warnings"`
	}
	if json.Unmarshal([]byte(text), &res) != nil {
		return
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s: %s\n", w.Code, w.Message)
	}
}

// revGetTools maps the types whose transitions name a rev (core T1010) to
// their get tool.
var revGetTools = map[string]string{
	"decision":      "get_decision",
	"amendment":     "get_amendment",
	"organization":  "get_organization",
	"domain":        "get_domain",
	"area":          "get_area",
	"coveragerule":  "get_coverage_rule",
	"coverage_rule": "get_coverage_rule",
}

// revGetTool returns the get tool of a type that names a rev, or "".
func revGetTool(typeName string) string {
	return revGetTools[strings.ToLower(typeName)]
}

// revTypes lists the types for the help text.
func revTypes() string {
	seen := map[string]bool{}
	var names []string
	for _, v := range revGetTools {
		if !seen[v] {
			seen[v] = true
			names = append(names, strings.TrimPrefix(v, "get_"))
		}
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
