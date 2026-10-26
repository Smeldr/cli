package main

import (
	"strings"
	"testing"
)

func TestAdministrators_ToolAndOutput(t *testing.T) {
	result := map[string]any{"administrators": []any{
		map[string]any{"name": "peter", "class": "human"},
		map[string]any{"name": "ops-bot", "class": "job"},
	}}
	tool, args, out := mockMCP(t, result, func() { runAdministratorsCommand(nil) })
	if tool != "administrators" || len(args) != 0 {
		t.Errorf("tool %q, args %v", tool, args)
	}
	for _, want := range []string{"NAME", "CLASS", "peter", "human", "ops-bot", "job"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	_, _, out = mockMCP(t, map[string]any{"administrators": []any{}}, func() { runAdministratorsCommand(nil) })
	if !strings.Contains(out, "nobody administers this instance") {
		t.Errorf("empty:\n%s", out)
	}
	_, _, out = mockMCP(t, result, func() { runAdministratorsCommand([]string{"--json"}) })
	if !strings.Contains(out, `"administrators"`) {
		t.Errorf("--json:\n%s", out)
	}
	_, _, out = mockMCP(t, result, func() { runAdministratorsCommand([]string{"--help"}) })
	if !strings.Contains(out, "who administers this instance") {
		t.Errorf("--help:\n%s", out)
	}
}
