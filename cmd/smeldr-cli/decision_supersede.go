package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// smeldr-cli decision supersede, retire and repair (core T1187, D189): each
// reads the Decision, shows it with its rev, asks, and makes one write.

// runDecisionSupersedeCommand is smeldr-cli decision supersede.
func runDecisionSupersedeCommand(args []string) {
	if wantsHelp(args) {
		printDecisionSupersedeHelp()
		return
	}
	args, rev, err := takeRev(args)
	if err != nil {
		fatal("%v", err)
		return
	}
	fs := newCommandFlags("decision supersede")
	fs.Usage = printDecisionSupersedeHelp
	by := fs.String("by", "", "an existing, ratified Decision that replaces it")
	newForm := fs.Bool("new", false, "create the replacement from --from")
	from := fs.String("from", "", "with --new: a file with title, scope (optional) and the basis as body, or - for stdin")
	reason := fs.String("reason", "", "why (required)")
	pos := parseCommandFlags(fs, args, 1, "decision supersede needs the Decision it replaces: address, slug or ID")
	decision := pos[0]
	switch {
	case strings.TrimSpace(*reason) == "":
		fatal("decision supersede needs --reason")
	case (*by != "") == *newForm:
		fatal("give exactly one of --by <decision> and --new --from <file>")
	case *newForm && *from == "":
		fatal("--new needs --from <file>: title, scope (optional) and the basis as body")
	}
	callArgs := map[string]any{"decision": decision, "reason": *reason}
	var newTitle string
	newSources := 0
	if *newForm {
		data, err := readFromFile(*from)
		if err != nil {
			fatal("%v", err)
		}
		fields, body, err := parseFrontmatter(string(data))
		if err != nil {
			fatal("%v", err)
		}
		newTitle, _ = fields["title"].(string)
		if strings.TrimSpace(newTitle) == "" || strings.TrimSpace(body) == "" {
			fatal("the file needs a title and the basis as body")
		}
		n := map[string]any{"title": newTitle, "body": body}
		if s, ok := fields["scope"].(string); ok && s != "" {
			n["scope"] = s
		}
		sources, err := frontmatterSources(string(data), *from)
		if err != nil {
			fatal("%v", err)
		}
		if sources != nil {
			n["sources"] = sources
			newSources = len(sources)
		}
		callArgs["new"] = n
	} else {
		callArgs["replacement"] = *by
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	if rev == nil {
		if *newForm {
			// The confirmation says what is about to hold, not only what moves.
			fmt.Fprintf(os.Stderr, "This creates a new Decision, %q, places it where %s is placed, and ratifies it,\n"+
				"in the same act that moves %s to superseded.\n", newTitle, decision, decision)
			if newSources > 0 {
				fmt.Fprintf(os.Stderr, "It is pinned to the %d sources in the file.\n", newSources)
			}
		} else {
			fmt.Fprintf(os.Stderr, "This records %s as the replacement of %s and moves %s to superseded, in one act.\n", *by, decision, decision)
		}
		n := readAndConfirm(cfg, "decision supersede", "Supersede", "get_decision", map[string]any{"slug": decision}, true)
		rev = &n
	}
	callArgs["rev"] = *rev
	text := writeCall(cfg, "supersede_decision", callArgs, nil)
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// runDecisionRetireCommand is smeldr-cli decision retire.
func runDecisionRetireCommand(args []string) {
	if wantsHelp(args) {
		printDecisionRetireHelp()
		return
	}
	args, rev, err := takeRev(args)
	if err != nil {
		fatal("%v", err)
		return
	}
	fs := newCommandFlags("decision retire")
	fs.Usage = printDecisionRetireHelp
	reason := fs.String("reason", "", "why (required)")
	pos := parseCommandFlags(fs, args, 1, "decision retire needs a Decision: address, slug or ID")
	decision := pos[0]
	if strings.TrimSpace(*reason) == "" {
		fatal("decision retire needs --reason")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	if rev == nil {
		fmt.Fprintf(os.Stderr, "This takes %s out of force with no replacement: it moves to retired.\n", decision)
		n := readAndConfirm(cfg, "decision retire", "Retire", "get_decision", map[string]any{"slug": decision}, true)
		rev = &n
	}
	text := writeCall(cfg, "retire_decision", map[string]any{"decision": decision, "reason": *reason, "rev": *rev}, nil)
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// runDecisionRepairCommand is smeldr-cli decision repair.
func runDecisionRepairCommand(args []string) {
	if wantsHelp(args) {
		printDecisionRepairHelp()
		return
	}
	args, rev, err := takeRev(args)
	if err != nil {
		fatal("%v", err)
		return
	}
	fs := newCommandFlags("decision repair")
	fs.Usage = printDecisionRepairHelp
	by := fs.String("by", "", "the Decision that replaced it")
	retire := fs.Bool("retire", false, "retire it instead")
	reason := fs.String("reason", "", "why (required)")
	pos := parseCommandFlags(fs, args, 1, "decision repair needs the superseded Decision: address, slug or ID")
	decision := pos[0]
	switch {
	case strings.TrimSpace(*reason) == "":
		fatal("decision repair needs --reason")
	case (*by != "") == *retire:
		fatal("give exactly one of --by <decision> and --retire")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	callArgs := map[string]any{"decision": decision, "reason": *reason}
	if *retire {
		callArgs["retire"] = true
	} else {
		callArgs["replacement"] = *by
	}
	if rev == nil {
		if *retire {
			fmt.Fprintf(os.Stderr, "This retires %s, superseded with no replacement recorded.\n", decision)
		} else {
			fmt.Fprintf(os.Stderr, "This records %s as the replacement of %s, superseded with no replacement recorded.\n", *by, decision)
		}
		n := readAndConfirm(cfg, "decision repair", "Repair", "get_decision", map[string]any{"slug": decision}, true)
		rev = &n
	}
	callArgs["rev"] = *rev
	text := writeCall(cfg, "repair_superseded", callArgs, nil)
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func printDecisionSupersedeHelp() {
	fmt.Print(`smeldr-cli decision supersede: replace a Decision in one act (core D189)

Usage:
  smeldr-cli decision supersede <decision> --by <replacement> --reason <text> [--rev N]
  smeldr-cli decision supersede <decision> --new --from <file> --reason <text> [--rev N]

--by names an existing, ratified Decision: it is recorded as the replacement
and <decision> moves to superseded. --new creates the replacement from the
file (title, scope optional, the basis as body), places it where <decision>
is placed and ratifies it, in the same act. The sources of <decision> are
not copied. To pin the new Decision's sources, give a sources line in the
front matter: a JSON array on that one line, or the path of a JSON file in
the format of decision sources --set (relative to the file):

  ---
  title: Use Postgres for the ledger
  sources: sources.json
  ---
  The basis.
 Needs ratify over the places; an agent token is refused.

Without --rev, on a terminal, cli shows the Decision and asks before it writes.
`)
}

func printDecisionRetireHelp() {
	fmt.Print(`smeldr-cli decision retire: take a Decision out of force without a replacement (core D189)

Usage:
  smeldr-cli decision retire <decision> --reason <text> [--rev N]

The Decision moves to retired, its own final state. Needs ratify over its
place; an agent token is refused. Without --rev, on a terminal, cli shows the
Decision and asks before it writes.
`)
}

func printDecisionRepairHelp() {
	fmt.Print(`smeldr-cli decision repair: repair a Decision superseded with no replacement recorded (core D189)

Usage:
  smeldr-cli decision repair <decision> --by <replacement> --reason <text> [--rev N]
  smeldr-cli decision repair <decision> --retire --reason <text> [--rev N]

--by records the Decision that replaced it (one that is, or was, in force);
--retire moves it to retired. Such Decisions are listed by smeldr-cli
findings (detector decision-replacement). Without --rev, on a terminal, cli
shows the Decision and asks before it writes.
`)
}

// readFromFile reads the file at path, or stdin when path is "-".
func readFromFile(path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	return data, nil
}

// frontmatterSources reads the sources line of a Decision file's front
// matter: a JSON array of sources on that one line, or the path of a JSON
// file in the format of decision sources --set (relative to the file's own
// directory). nil when the front matter has no sources line. The front
// matter parser splits lists on commas, so the line is read here, raw.
func frontmatterSources(content, from string) ([]decisionSource, error) {
	header, ok := strings.CutPrefix(content, "---\n")
	if !ok {
		return nil, nil
	}
	if end := strings.Index(header, "\n---"); end >= 0 {
		header = header[:end]
	}
	for _, line := range strings.Split(header, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), "sources") {
			continue
		}
		v = strings.TrimSpace(v)
		raw := []byte(v)
		if !strings.HasPrefix(v, "[") {
			p := v
			if !filepath.IsAbs(p) && from != "-" {
				p = filepath.Join(filepath.Dir(from), p)
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil, fmt.Errorf("sources: read %q: %w", p, err)
			}
			raw = data
		}
		sources := []decisionSource{}
		if err := json.Unmarshal(raw, &sources); err != nil {
			return nil, fmt.Errorf("sources must be a JSON array of {repository, commit, path, anchor, role, hash}, on the sources line or in the file it names: %v", err)
		}
		return sources, nil
	}
	return nil, nil
}
