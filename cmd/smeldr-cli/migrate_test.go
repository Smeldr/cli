package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var migrateReport = map[string]any{
	"dry_run":     true,
	"duration_ms": 12,
	"types": []map[string]any{
		{"type_name": "Decision", "prefix": "D", "kept": 3, "renumbered": 1, "newly_numbered": 0},
		{"type_name": "Signal", "prefix": "S", "kept": 0, "renumbered": 0, "newly_numbered": 5},
		{"type_name": "Run", "prefix": "R", "kept": 0, "renumbered": 0, "newly_numbered": 0},
	},
	"rewritten":  []any{},
	"left_alone": []any{},
}

func TestMigrateAddresses_DryRunByDefault(t *testing.T) {
	tool, args, out := mockMCP(t, migrateReport, func() {
		runMigrateAddressesCommand(nil)
	})
	if tool != "migrate_addresses" {
		t.Errorf("tool = %q", tool)
	}
	if _, sent := args["apply"]; sent {
		t.Errorf("args = %v; a dry run sends no apply", args)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil || got["dry_run"] != true {
		t.Errorf("stdout is not the JSON report: %v\n%s", err, out)
	}
}

func TestMigrateAddresses_Apply(t *testing.T) {
	_, args, _ := mockMCP(t, migrateReport, func() {
		runMigrateAddressesCommand([]string{"--apply"})
	})
	if args["apply"] != true {
		t.Errorf("args = %v; want apply true", args)
	}
}

func TestMigrateAddresses_ReportFile(t *testing.T) {
	for _, form := range []string{"equals", "space"} {
		t.Run(form, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "report.json")
			argv := []string{"--report=" + path}
			if form == "space" {
				argv = []string{"--report", path}
			}
			_, _, out := mockMCP(t, migrateReport, func() { runMigrateAddressesCommand(argv) })
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(b)) != strings.TrimSpace(out) {
				t.Errorf("file and stdout differ:\nfile %s\nstdout %s", b, out)
			}
			if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
				t.Errorf("leftover files: %v", entries)
			}
		})
	}
}

func TestParseMigrateArgs(t *testing.T) {
	ok := []struct {
		args []string
		want migrateArgs
	}{
		{nil, migrateArgs{}},
		{[]string{"--apply"}, migrateArgs{apply: true}},
		{[]string{"--report", "r.json", "--apply"}, migrateArgs{apply: true, report: "r.json"}},
		{[]string{"--report=r.json"}, migrateArgs{report: "r.json"}},
		{[]string{"--help"}, migrateArgs{help: true}},
	}
	for _, c := range ok {
		got, err := parseMigrateArgs(c.args)
		if err != nil || got != c.want {
			t.Errorf("parseMigrateArgs(%v) = %+v, %v; want %+v", c.args, got, err, c.want)
		}
	}
	bad := [][]string{
		{"--report"},
		{"--report", "--apply"},
		{"--report="},
		{"--dry-run"},
		{"-x"},
		{"Decision"},
	}
	for _, args := range bad {
		if _, err := parseMigrateArgs(args); err == nil {
			t.Errorf("parseMigrateArgs(%v) accepted", args)
		}
	}
}

func TestMigrateSummary(t *testing.T) {
	raw, _ := json.Marshal(migrateReport)
	got := migrateSummary(raw)
	for _, want := range []string{"dry run, nothing written", "Decision 4", "Signal 5", "renumbered 1", "12 ms"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q lacks %q", got, want)
		}
	}
	if strings.Contains(got, "Run") {
		t.Errorf("summary %q names a type with nothing numbered", got)
	}
	if got := migrateSummary([]byte(`{"dry_run":false,"types":[]}`)); !strings.Contains(got, "applied") || !strings.Contains(got, "numbered none") {
		t.Errorf("applied summary = %q", got)
	}
	if got := migrateSummary([]byte("not json")); !strings.Contains(got, "unexpected") {
		t.Errorf("bad report summary = %q", got)
	}
}

func TestWriteReport_Errors(t *testing.T) {
	dir := t.TempDir()
	if err := writeReport(filepath.Join(dir, "missing", "r.json"), []byte(`{}`)); err == nil {
		t.Error("a report in a missing directory was written")
	}
	if err := writeReport(filepath.Join(dir, "r.json"), []byte("not json")); err == nil {
		t.Error("a non-JSON report was written")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("leftover files: %v", entries)
	}
}

func TestMigrateAddresses_Help(t *testing.T) {
	out := captureStdout(t, func() { runMigrateAddressesCommand([]string{"--help"}) })
	for _, want := range []string{"smeldr-cli migrate-addresses", "dry run", "--apply", "Admin", "smeldr.dev/mcp v1.60.0", "keep the report"} {
		if !strings.Contains(out, want) {
			t.Errorf("help is missing %q:\n%s", want, out)
		}
	}
}
