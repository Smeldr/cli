package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testCommit = "0123456789abcdef0123456789abcdef01234567"

func TestDecisionSources_List(t *testing.T) {
	result := map[string]any{"address": "D148", "sources": []any{
		map[string]any{"repository": "acme/app", "commit": testCommit, "path": "docs/money.md", "anchor": "## Cents", "role": "source"},
	}}
	tool, args, out := mockMCP(t, result, func() { runDecisionCommand([]string{"sources", "D148"}) })
	if tool != "get_decision" || args["slug"] != "D148" {
		t.Errorf("tool %q, args %v", tool, args)
	}
	for _, want := range []string{"ROLE", "source", "acme/app", "0123456789ab", "docs/money.md", "## Cents"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	_, _, out = mockMCP(t, map[string]any{"sources": []any{}}, func() { runDecisionCommand([]string{"sources", "D9"}) })
	if !strings.Contains(out, "D9 has no sources") {
		t.Errorf("empty:\n%s", out)
	}
	_, _, out = mockMCP(t, result, func() { runDecisionCommand([]string{"sources", "D148", "--json"}) })
	if !strings.Contains(out, `"path": "docs/money.md"`) {
		t.Errorf("--json:\n%s", out)
	}
	_, _, out = mockMCP(t, result, func() { runDecisionCommand([]string{"sources", "--help"}) })
	if !strings.Contains(out, "pinned external sources") {
		t.Errorf("--help:\n%s", out)
	}
}

func TestDecisionSources_Set(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sources.json")
	body := fmt.Sprintf(`[{"repository":"acme/app","commit":%q,"path":"docs/a.md","role":"evidence"}]`, testCommit)
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	tool, args, out := mockMCP(t, map[string]any{}, func() { runDecisionCommand([]string{"sources", "D148", "--set", file}) })
	list, _ := args["sources"].([]any)
	if tool != "update_decision" || args["slug"] != "D148" || len(list) != 1 || !strings.Contains(out, "1 sources set") {
		t.Errorf("tool %q, args %v, out %s", tool, args, out)
	}
	stdin = strings.NewReader(body)
	old := stdin
	t.Cleanup(func() { stdin = old })
	_, args, _ = mockMCP(t, map[string]any{}, func() { runDecisionCommand([]string{"sources", "D148", "--set", "-"}) })
	if list, _ := args["sources"].([]any); len(list) != 1 {
		t.Errorf("stdin: %v", args)
	}
	_, args, out = mockMCP(t, map[string]any{}, func() { runDecisionCommand([]string{"sources", "D148", "--clear"}) })
	if list, ok := args["sources"].([]any); !ok || len(list) != 0 || !strings.Contains(out, "0 sources set") {
		t.Errorf("--clear: %v %s", args, out)
	}
}
