package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

// runAuditCommand dispatches audit subcommands. args begins with the verb.
func runAuditCommand(args []string) {
	if len(args) == 0 {
		printAuditHelp()
		os.Exit(1)
	}
	switch args[0] {
	case "-h", "--help", "help":
		printAuditHelp()
	case "list":
		runAuditList(args[1:])
	default:
		fatal("unknown audit verb %q — use: list", args[0])
	}
}

func printAuditHelp() {
	fmt.Fprint(os.Stdout, `forge-cli audit — audit trail (Editor role required)

Verbs:
  list [flags]   show audit records, newest first

Flags for list:
  --from <RFC3339>   filter records at or after this timestamp
  --to   <RFC3339>   filter records at or before this timestamp
  --type <string>    filter by content type (e.g. "Post")
  --actor <string>   filter by actor ID

The audit endpoint (GET /_audit) is called directly (not via MCP).
`)
}

// auditRecord mirrors forge.AuditRecord for JSON decoding.
type auditRecord struct {
	ID            string    `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	Signal        string    `json:"signal"`
	ContentType   string    `json:"content_type"`
	Slug          string    `json:"slug"`
	ActorID       string    `json:"actor_id"`
	ActorRole     string    `json:"actor_role"`
	PreviousState string    `json:"previous_state"`
}

// runAuditList fetches audit records from GET /_audit and prints a table.
func runAuditList(args []string) {
	fs := flag.NewFlagSet("audit list", flag.ExitOnError)
	fromFlag := fs.String("from", "", "filter records at or after this RFC3339 timestamp")
	toFlag := fs.String("to", "", "filter records at or before this RFC3339 timestamp")
	typeFlag := fs.String("type", "", "filter by content type")
	actorFlag := fs.String("actor", "", "filter by actor ID")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: forge-cli audit list [--from RFC3339] [--to RFC3339] [--type TYPE] [--actor ACTOR]")
	}
	fs.Parse(args) //nolint:errcheck

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}

	q := url.Values{}
	if *fromFlag != "" {
		if _, err := time.Parse(time.RFC3339, *fromFlag); err != nil {
			fatal("--from must be an RFC3339 timestamp (e.g. 2026-01-01T00:00:00Z)")
		}
		q.Set("from", *fromFlag)
	}
	if *toFlag != "" {
		if _, err := time.Parse(time.RFC3339, *toFlag); err != nil {
			fatal("--to must be an RFC3339 timestamp (e.g. 2026-12-31T23:59:59Z)")
		}
		q.Set("to", *toFlag)
	}
	if *typeFlag != "" {
		q.Set("type", *typeFlag)
	}
	if *actorFlag != "" {
		q.Set("actor", *actorFlag)
	}

	endpoint := cfg.ForgeURL + "/_audit"
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}

	raw, code, err := request(cfg, http.MethodGet, endpoint, nil)
	if err != nil {
		fatal("request failed: %v", err)
	}
	if code >= 400 {
		fatal("/_audit returned %d: %s", code, strings.TrimSpace(string(raw)))
	}

	var records []auditRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		fatal("decode response: %v", err)
	}

	if len(records) == 0 {
		fmt.Println("no audit records found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TIMESTAMP\tSIGNAL\tTYPE\tSLUG\tACTOR\tPREV STATE")
	for _, r := range records {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Timestamp.UTC().Format("2006-01-02 15:04:05"),
			r.Signal,
			r.ContentType,
			r.Slug,
			r.ActorID,
			r.PreviousState,
		)
	}
	w.Flush()
}
