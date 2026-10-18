package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestSearch_ToolAndArgs(t *testing.T) {
	result := map[string]any{"total": 3, "count": 2, "hits": []any{
		map[string]any{"type": "Decision", "id": "d1", "address": "D7", "title": "Use apples", "state": "proposed", "snippet": "about apples"},
		map[string]any{"type": "note", "id": "n1", "title": "Shopping", "state": "draft"},
	}}
	tool, args, out := mockMCP(t, result, func() {
		runSearchCommand([]string{"apple pie", "--type", "Decision", "--type", "note", "--state", "proposed", "--place", "none", "--limit", "2", "--offset", "1"})
	})
	if tool != "search" {
		t.Errorf("tool = %q, want search", tool)
	}
	if args["query"] != "apple pie" || args["place"] != "none" || fmt.Sprint(args["types"]) != "[Decision note]" ||
		fmt.Sprint(args["states"]) != "[proposed]" || args["limit"] != float64(2) || args["offset"] != float64(1) {
		t.Errorf("args = %v", args)
	}
	for _, want := range []string{"RECORD", "D7", "Use apples", "about apples", "n1", "2 of 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}

func TestSearch_NoHitsAndJSON(t *testing.T) {
	_, args, out := mockMCP(t, map[string]any{"total": 0, "hits": []any{}}, func() {
		runSearchCommand([]string{"--place", "DOM1"})
	})
	if _, ok := args["query"]; ok || strings.TrimSpace(out) != "no hits (total 0)" {
		t.Errorf("args %v, output %q", args, out)
	}
	_, _, out = mockMCP(t, map[string]any{"total": 0, "hits": []any{}}, func() {
		runSearchCommand([]string{"apple", "--json"})
	})
	if !strings.Contains(out, `"total"`) {
		t.Errorf("--json should print the whole result:\n%s", out)
	}
}

func TestSearch_Help(t *testing.T) {
	out := captureStdout(t, func() { runSearchCommand([]string{"--help"}) })
	for _, want := range []string{"smeldr-cli search", "--place none", "never count", "v1.65.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("help is missing %q:\n%s", want, out)
		}
	}
}
