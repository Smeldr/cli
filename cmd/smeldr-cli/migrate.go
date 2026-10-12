package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// migrateArgs is the parsed command line of migrate-addresses.
type migrateArgs struct {
	apply  bool
	report string
	help   bool
}

// parseMigrateArgs parses migrate-addresses' arguments. It never guesses: an
// unknown flag, a positional argument or --report without a path is an error.
func parseMigrateArgs(args []string) (migrateArgs, error) {
	var m migrateArgs
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help" || a == "help":
			m.help = true
		case a == "--apply":
			m.apply = true
		case a == "--report":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return m, fmt.Errorf("--report needs a path")
			}
			i++
			m.report = args[i]
		case strings.HasPrefix(a, "--report="):
			m.report = strings.TrimPrefix(a, "--report=")
			if m.report == "" {
				return m, fmt.Errorf("--report needs a path")
			}
		case strings.HasPrefix(a, "-"):
			return m, fmt.Errorf("unknown flag %q", a)
		default:
			return m, fmt.Errorf("migrate-addresses takes no argument, got %q", a)
		}
	}
	return m, nil
}

// runMigrateAddressesCommand runs D110's address migration on the instance via
// the migrate_addresses MCP tool: a dry run unless --apply is given. The report
// goes to stdout as JSON, a one-line summary to stderr, and with --report also
// to a file.
func runMigrateAddressesCommand(args []string) {
	m, err := parseMigrateArgs(args)
	if err != nil {
		fatal("%v (see smeldr-cli migrate-addresses --help)", err)
	}
	if m.help {
		printMigrateAddressesHelp()
		return
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	toolArgs := map[string]any{}
	if m.apply {
		toolArgs["apply"] = true
	}
	text, err := mcpCall(cfg, "migrate_addresses", toolArgs)
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
	fmt.Fprintln(os.Stderr, migrateSummary([]byte(text)))
	if m.report != "" {
		if err := writeReport(m.report, []byte(text)); err != nil {
			fatal("write report: %v", err)
		}
		fmt.Fprintf(os.Stderr, "report written to %s\n", m.report)
	}
}

// migrateSummary is the one-line summary of a migration report.
func migrateSummary(raw []byte) string {
	var rep struct {
		DryRun     bool  `json:"dry_run"`
		DurationMS int64 `json:"duration_ms"`
		Types      []struct {
			TypeName      string `json:"type_name"`
			Kept          int    `json:"kept"`
			Renumbered    int    `json:"renumbered"`
			NewlyNumbered int    `json:"newly_numbered"`
		} `json:"types"`
	}
	if err := json.Unmarshal(raw, &rep); err != nil {
		return "migrate-addresses: unexpected report"
	}
	mode := "applied"
	if rep.DryRun {
		mode = "dry run, nothing written"
	}
	var parts []string
	renumbered := 0
	for _, t := range rep.Types {
		if n := t.Kept + t.Renumbered + t.NewlyNumbered; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", t.TypeName, n))
		}
		renumbered += t.Renumbered
	}
	numbered := "none"
	if len(parts) > 0 {
		numbered = strings.Join(parts, ", ")
	}
	return fmt.Sprintf("migrate-addresses (%s): numbered %s; renumbered %d; writer held %d ms", mode, numbered, renumbered, rep.DurationMS)
}

// writeReport writes the report indented to path through a temporary file in
// the same directory and a rename, so a failed write never leaves half a report.
func writeReport(path string, raw []byte) error {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("the report is not JSON: %w", err)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".migrate-report-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(append(out, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

func printMigrateAddressesHelp() {
	fmt.Print(`smeldr-cli migrate-addresses: give every record without an address its address (D110)

Usage:
  smeldr-cli migrate-addresses [--apply] [--report <path>]

Without --apply this is a dry run: the server runs the whole migration in one
transaction and rolls it back, so the report shows exactly what an apply would
do and nothing is written. --apply writes it. A second apply changes nothing.

The report is printed to stdout as JSON: per type the prefix, counts before
and after, every renumbered record with its old and new address, every
rewritten reference and every reference left alone, and duration_ms, how long
the database writer was held. A one-line summary goes to stderr. --report also
writes the report to a file.

Applying is a governance act: run a dry run first, read the report, back up
the database, and keep the report with the deploy record (renumbered Decisions
are part of the governance record, and the instance stores no copy).

Requires the Admin role (the administer operation under governance) and
smeldr.dev/mcp v1.60.0 or later with smeldr.dev/core v1.145.0 or later on the
server.

Example:
  smeldr-cli migrate-addresses --report dry.json
  smeldr-cli migrate-addresses --apply --report apply.json
`)
}
