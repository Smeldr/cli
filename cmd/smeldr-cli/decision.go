package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"text/tabwriter"
)

// runDecisionCommand handles smeldr-cli decision sources (core T914, D148);
// any other subcommand is the generic content command on the Decisions route.
// The content verbs take the URL path segment, and core mounts Decisions at
// /decisions, so the segment is plural (T1122).
func runDecisionCommand(args []string) {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		printDecisionHelp()
		return
	}
	if len(args) > 0 {
		switch args[0] {
		case "sources":
			runDecisionSourcesCommand(args[1:])
			return
		case "supersede":
			runDecisionSupersedeCommand(args[1:])
			return
		case "retire":
			runDecisionRetireCommand(args[1:])
			return
		case "repair":
			runDecisionRepairCommand(args[1:])
			return
		case "place":
			runDecisionPlace(args[1:])
			return
		case "get":
			runDecisionGet(args[1:])
			return
		case "create", "update":
			if wantsHelp(args[1:]) {
				printDecisionFileHelp(args[0])
				return
			}
		}
	}
	runContentCommand("decisions", args)
}

// printDecisionFileHelp is decision create --help and decision update --help:
// the command as it is typed, and the file's form.
func printDecisionFileHelp(verb string) {
	usage := "smeldr-cli decision create --from <file>"
	if verb == "update" {
		usage = "smeldr-cli decision update <slug> --from <file>"
	}
	fmt.Printf(`Usage: %s

The file (or - for stdin) is front matter, then the basis as the body:

  ---
  title: Money is stored in cents
  scope: billing
  ---
  Why: the reasons, in Markdown, below the closing ---.

title is required on create (one line, at most 120 characters); scope is
optional. On update, fields the file leaves out keep their stored value.
CRLF and lone CR line endings are read as LF.
`, usage)
}

// runDecisionGet prints one Decision. A slug the /decisions route finds is
// read there, as before, so every call that works today answers the same; an
// address or an ID, which the route does not find, is read with get_decision.
func runDecisionGet(args []string) {
	fs := newCommandFlags("decision get")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: smeldr-cli decision get <decision>   (an address such as D4, a slug or an ID)")
	}
	ident := parseCommandFlags(fs, args, 1, "decision get requires a Decision: address, slug or ID")[0]
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	raw, code, err := request(cfg, http.MethodGet, cfg.ForgeURL+"/decisions/"+url.PathEscape(ident), nil)
	if err != nil {
		fatal("%v", err)
	}
	if code == http.StatusNotFound {
		text, err := mcpCall(cfg, "get_decision", map[string]any{"slug": ident})
		if err != nil {
			fatal("decision %s: /decisions did not find it, and get_decision answered: %v", ident, err)
			return
		}
		raw, code = []byte(text), http.StatusOK
	}
	if code >= 400 {
		prefixFailure("decisions", "get", code, raw)
		return
	}
	if err := printJSON(raw); err != nil {
		fatal("%v", err)
	}
}

// printDecisionHelp is smeldr-cli decision --help (T1147): the content verbs
// on the /decisions route, and sources.
func printDecisionHelp() {
	fmt.Print(`smeldr-cli decision: Decisions

Usage:
  smeldr-cli decision create --from <file>          propose a Decision (title, scope, body from the file)
  smeldr-cli decision update <slug> --from <file>   update fields (absent fields preserved)
  smeldr-cli decision get <decision>                print one Decision as JSON (address, slug or ID)
  smeldr-cli decision list [--status <s>] [--json]  list Decisions
  smeldr-cli decision place <decision> (--area <a> | --domain <d> | --organization <o>) [--rev N]
                                                   put it in its place
  smeldr-cli decision sources <decision> [--set f.json|--clear] [--rev N]
                                                   a Decision's pinned sources
  smeldr-cli decision supersede <decision> (--by <d> | --new --from <file>) --reason <text> [--rev N]
                                                   replace it in one act
  smeldr-cli decision retire <decision> --reason <text> [--rev N]
                                                   take it out of force, no replacement
  smeldr-cli decision repair <decision> (--by <d> | --retire) --reason <text> [--rev N]
                                                   repair one superseded with no replacement

A Decision starts proposed and is placed before it is ratified; ratifying it is
smeldr-cli transition Decision <slug> --to ratified.
Replace it with decision supersede, never with transition --to superseded.
See smeldr-cli decision create --help for the file, and decision sources --help
for the sources file format.
`)
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
	args, rev, err := takeRev(args)
	if err != nil {
		fatal("%v", err)
		return
	}
	fs := newCommandFlags("decision sources")
	fs.Usage = printDecisionSourcesHelp
	asJSON := fs.Bool("json", false, "raw JSON")
	clearFlag := fs.Bool("clear", false, "remove every source")
	set := fs.String("set", "", "a JSON file of sources, or - for stdin")
	pos := parseCommandFlags(fs, args, 1, "decision sources needs a Decision: address, slug or ID")
	decision, setFrom, raw, clear := pos[0], *set, *asJSON, *clearFlag
	if setFlags(fs)["set"] && setFrom == "" {
		fatal("--set needs a JSON file, or - for stdin")
	}
	if decision == "" {
		fatal("decision sources needs a Decision: address, slug or ID")
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
			data, err := readFromFile(setFrom)
			if err != nil {
				fatal("read sources: %v", err)
			}
			if err := json.Unmarshal(data, &sources); err != nil {
				fatal("sources must be a JSON array of {repository, commit, path, anchor, role, hash}: %v", err)
			}
		}
		// The rev of the Decision the person read (core T1010).
		if rev == nil {
			n := readAndConfirm(cfg, "decision sources --set", "Replace the sources of", "get_decision", map[string]any{"slug": decision}, false)
			rev = &n
		}
		writeCall(cfg, "update_decision", map[string]any{"slug": decision, "sources": sources, "rev": *rev}, nil)
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
	fmt.Print(`smeldr-cli decision sources: a Decision's pinned external sources

Usage:
  smeldr-cli decision sources <decision> [--json]
  smeldr-cli decision sources <decision> --set <file.json|->
  smeldr-cli decision sources <decision> --clear

Each source is {repository: owner/name, commit: a full 40-character SHA (never
a branch), path: one file, anchor: optional, role: source|evidence|implementation,
hash: optional sha256:<64 hex>}; at most 20. --set replaces the whole list.
Sources are set while the Decision is proposed: a ratified Decision is locked,
so to re-pin it, propose a Decision that supersedes it. Needs smeldr.dev/core
with Decision sources on the server.
`)
}
