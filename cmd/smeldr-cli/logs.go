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

// logsEnvelope mirrors the GET /_logs response envelope from core v1.36.0 (A128).
type logsEnvelope struct {
	Capacity int        `json:"capacity"`
	Count    int        `json:"count"`
	Dropped  uint64     `json:"dropped"`
	Entries  []logEntry `json:"entries"`
}

// logEntry mirrors smeldr.LogEntry for JSON decoding.
type logEntry struct {
	Time  time.Time      `json:"time"`
	Level string         `json:"level"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs"`
	Seq   uint64         `json:"seq"`
}

// runLogsCommand fetches the in-memory log ring from GET /_logs and prints
// a human-readable table (default) or raw JSON (--json). Requires Admin role.
// The endpoint is called directly (not via MCP) so it works when MCP is down.
func runLogsCommand(args []string) {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	levelFlag := fs.String("level", "", "minimum log level inclusive (e.g. warn, error)")
	limitFlag := fs.Int("limit", 0, "most recent N entries (0 = no limit)")
	sinceFlag := fs.String("since", "", "entries strictly after this RFC3339 timestamp")
	jsonFlag := fs.Bool("json", false, "print raw JSON response")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: smeldr-cli logs [--level LEVEL] [--limit N] [--since RFC3339] [--json]")
		fmt.Fprintln(os.Stderr, "Requires Admin role. The server must call app.CaptureLogs() (core v1.36.0+).")
	}
	fs.Parse(args) //nolint:errcheck

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}

	if *sinceFlag != "" {
		if _, err := time.Parse(time.RFC3339, *sinceFlag); err != nil {
			fatal("--since must be an RFC3339 timestamp (e.g. 2026-01-01T00:00:00Z)")
		}
	}

	q := url.Values{}
	if *levelFlag != "" {
		q.Set("level", *levelFlag)
	}
	if *limitFlag > 0 {
		q.Set("limit", fmt.Sprintf("%d", *limitFlag))
	}
	if *sinceFlag != "" {
		q.Set("since", *sinceFlag)
	}

	endpoint := cfg.ForgeURL + "/_logs"
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}

	raw, code, err := request(cfg, http.MethodGet, endpoint, nil)
	if err != nil {
		fatal("request failed: %v", err)
	}
	switch code {
	case http.StatusUnauthorized:
		fatal("Admin token required")
	case http.StatusForbidden:
		fatal("forbidden — Admin role required")
	case http.StatusNotFound:
		fatal("/_logs not available — call app.CaptureLogs() on the server (core v1.36.0+)")
	}
	if code >= 400 {
		fatal("/_logs returned %d: %s", code, strings.TrimSpace(string(raw)))
	}

	if *jsonFlag {
		printJSON(raw)
		return
	}

	var env logsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		fatal("decode response: %v", err)
	}

	if len(env.Entries) == 0 {
		fmt.Println("no log entries")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TIMESTAMP\tLEVEL\tSEQ\tMESSAGE")
	for _, e := range env.Entries {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
			e.Time.UTC().Format("2006-01-02 15:04:05"),
			strings.ToUpper(e.Level),
			e.Seq,
			e.Msg,
		)
	}
	w.Flush()

	if env.Dropped > 0 {
		fmt.Fprintf(os.Stdout, "(%d entries dropped — ring buffer overflowed)\n", env.Dropped)
	}
}
