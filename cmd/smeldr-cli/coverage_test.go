package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestDecisionsForPaths_ToolAndArgs(t *testing.T) {
	stdin = strings.NewReader("cmd/x/main.go\n\n docs/a.md \n")
	old := stdin
	t.Cleanup(func() { stdin = old })
	result := map[string]any{
		"areas":     []any{map[string]any{"address": "ARE1", "name": "Billing", "approval_mode": "comment-only", "paths": []any{"internal/billing/a.go"}}},
		"decisions": []any{map[string]any{"address": "D7", "title": "Money in cents", "place": "area", "place_address": "ARE1"}},
		"uncovered": []any{"docs/a.md"},
	}
	tool, args, out := mockMCP(t, result, func() {
		runDecisionsForPathsCommand([]string{"--repo", "acme/app", "internal/billing/a.go", "--stdin", "--no-organization"})
	})
	if tool != "decisions_for_paths" || args["repository"] != "acme/app" || args["include_organization"] != false ||
		fmt.Sprint(args["paths"]) != "[internal/billing/a.go cmd/x/main.go docs/a.md]" {
		t.Errorf("tool %q, args %v", tool, args)
	}
	for _, want := range []string{"ARE1", "Billing", "comment-only", "D7", "area ARE1", "Money in cents", "1 Decisions in force; 1 paths uncovered"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	_, _, out = mockMCP(t, map[string]any{"areas": []any{}, "decisions": []any{}, "uncovered": []any{"a"}}, func() {
		runDecisionsForPathsCommand([]string{"--repo", "acme/app", "a"})
	})
	if !strings.Contains(out, "no Area covers these paths") {
		t.Errorf("no areas:\n%s", out)
	}
	_, _, out = mockMCP(t, result, func() { runDecisionsForPathsCommand([]string{"--repo", "acme/app", "a", "--json"}) })
	if !strings.Contains(out, `"uncovered"`) {
		t.Errorf("--json:\n%s", out)
	}
}

func TestCoverageCheck_ToolAndArgs(t *testing.T) {
	stdin = strings.NewReader("README.md\ninternal/billing/a.go\n")
	old := stdin
	t.Cleanup(func() { stdin = old })
	result := map[string]any{"paths": 2, "rules": []any{
		map[string]any{"rule": "COV1", "area": "ARE1", "pattern": "internal/billing/**", "matches": 1},
		map[string]any{"rule": "COV2", "area": "ARE1", "pattern": "legacy/**", "matches": 0},
	}, "matching_nothing": []any{"COV2"}}
	tool, args, out := mockMCP(t, result, func() { runCoverageCommand([]string{"check", "--repo", "acme/app", "--stdin"}) })
	if tool != "check_coverage" || args["repository"] != "acme/app" || fmt.Sprint(args["paths"]) != "[README.md internal/billing/a.go]" {
		t.Errorf("tool %q, args %v", tool, args)
	}
	for _, want := range []string{"RULE", "COV1", "legacy/**", "2 paths, 2 rules, 1 matching nothing"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	_, _, out = mockMCP(t, result, func() { runCoverageCommand([]string{"check", "--repo", "acme/app", "a", "--json"}) })
	if !strings.Contains(out, `"matching_nothing"`) {
		t.Errorf("--json:\n%s", out)
	}
}

func TestCoverage_Help(t *testing.T) {
	for _, run := range []func(){
		func() { runDecisionsForPathsCommand([]string{"--help"}) },
		func() { runCoverageCommand(nil) },
	} {
		out := captureStdout(t, run)
		if !strings.Contains(out, "--repo owner/name") || !strings.Contains(out, "--stdin") {
			t.Errorf("help:\n%s", out)
		}
	}
}
