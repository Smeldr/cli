package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"
	"time"
)

// runFindingsCommand lists recorded findings via the list_findings MCP tool
// (core T953).
func runFindingsCommand(args []string) {
	params := map[string]any{}
	raw := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() string {
			if i+1 >= len(args) {
				fatal("%s needs a value", a)
			}
			i++
			return args[i]
		}
		switch a {
		case "-h", "--help", "help":
			printFindingsHelp()
			return
		case "--json":
			raw = true
		case "--detector":
			params["detector"] = next()
		case "--type":
			params["subject_type"] = next()
		case "--limit":
			n, err := strconv.Atoi(next())
			if err != nil {
				fatal("--limit must be a number")
			}
			params["limit"] = n
		default:
			fatal("findings: unknown argument %q", a)
		}
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "list_findings", params)
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
		Findings []struct {
			Detector    string    `json:"detector"`
			SubjectType string    `json:"subject_type"`
			SubjectID   string    `json:"subject_id"`
			Address     string    `json:"address"`
			Message     string    `json:"message"`
			LastSeenAt  time.Time `json:"last_seen_at"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		fatal("unexpected response: %v", err)
	}
	if len(res.Findings) == 0 {
		fmt.Println("no findings")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DETECTOR\tSUBJECT\tLAST SEEN\tMESSAGE")
	for _, f := range res.Findings {
		subject := f.Address
		if subject == "" {
			subject = f.SubjectType + ":" + f.SubjectID
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", f.Detector, subject, f.LastSeenAt.UTC().Format("2006-01-02 15:04"), f.Message)
	}
	w.Flush()
}

func printFindingsHelp() {
	fmt.Print(`smeldr-cli findings: what the detectors found

Usage:
  smeldr-cli findings [--detector D] [--type T] [--limit N] [--json]

Lists recorded findings newest first: detector (structural, eval-queue,
standing-drift, ...), subject (its address, or type:id), when last seen, and
the message. You see a finding only when you may read its subject. Findings
resolve when their detector stops firing; nothing here dismisses one.
--limit is 1 to 500 (default 100). Needs smeldr.dev/mcp with list_findings
(core T953) on the server.
`)
}
