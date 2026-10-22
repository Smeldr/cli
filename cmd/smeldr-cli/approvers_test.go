package main

import (
	"strings"
	"testing"
)

func TestApprovers_ToolAndArgs(t *testing.T) {
	result := map[string]any{"place": "ARE3", "approvers": []any{
		map[string]any{"name": "peter", "role": "approver", "class": "human", "anchor_address": ""},
		map[string]any{"name": "billing-bot", "role": "area-approver", "class": "agent", "anchor_address": "ARE3"},
	}}
	tool, args, out := mockMCP(t, result, func() { runApproversCommand([]string{"ARE3"}) })
	if tool != "approvers_of" || args["place"] != "ARE3" {
		t.Errorf("tool %q, args %v", tool, args)
	}
	for _, want := range []string{"NAME", "peter", "approver", "human", "-", "billing-bot", "area-approver", "ARE3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	_, _, out = mockMCP(t, map[string]any{"place": "DOM2", "approvers": []any{}}, func() { runApproversCommand([]string{"DOM2"}) })
	if !strings.Contains(out, "nobody approves DOM2") {
		t.Errorf("empty:\n%s", out)
	}
	_, _, out = mockMCP(t, result, func() { runApproversCommand([]string{"--json", "ARE3"}) })
	if !strings.Contains(out, `"approvers"`) {
		t.Errorf("--json:\n%s", out)
	}
	_, _, out = mockMCP(t, result, func() { runApproversCommand([]string{"--help"}) })
	if !strings.Contains(out, "who approves a place") {
		t.Errorf("--help:\n%s", out)
	}
}
