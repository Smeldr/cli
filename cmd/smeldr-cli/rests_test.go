package main

import (
	"strings"
	"testing"
)

func TestRests_Live(t *testing.T) {
	result := map[string]any{"type": "Decision", "id": "d1",
		"rests_on_this": map[string]any{"open": 2, "closed": 12, "at_least": false},
		"this_rests_on": map[string]any{"open": 0, "closed": 1000, "at_least": true}}
	tool, args, out := mockMCP(t, result, func() { runRestsCommand([]string{"Decision", "D12"}) })
	if tool != "get_rest_counts" || args["type"] != "Decision" || args["id"] != "D12" {
		t.Errorf("tool %q, args %v", tool, args)
	}
	for _, want := range []string{"rests on this: 2 open, 12 closed", "this rests on: at least 0 open, 1000 closed"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	_, _, out = mockMCP(t, result, func() { runRestsCommand([]string{"Decision", "D12", "--json"}) })
	if !strings.Contains(out, `"rests_on_this"`) {
		t.Errorf("--json:\n%s", out)
	}
	_, _, out = mockMCP(t, result, func() { runRestsCommand([]string{"--help"}) })
	if !strings.Contains(out, "what rests on an item") {
		t.Errorf("--help:\n%s", out)
	}
}

func TestRests_Stored(t *testing.T) {
	result := map[string]any{"items": []any{
		map[string]any{"type": "Decision", "id": "d1", "rests_on_this": map[string]any{"open": 1, "closed": 2}, "this_rests_on": map[string]any{}, "counted_at": "2026-10-23T12:00:00Z"},
		map[string]any{"type": "Decision", "id": "d2", "rests_on_this": map[string]any{}, "this_rests_on": map[string]any{}},
	}}
	tool, args, out := mockMCP(t, result, func() { runRestsCommand([]string{"--stored", "Decision:D1", "Decision:D2"}) })
	items, _ := args["items"].([]any)
	if tool != "list_rest_counts" || len(items) != 2 {
		t.Errorf("tool %q, args %v", tool, args)
	}
	for _, want := range []string{"ITEM", "Decision d1", "1 open, 2 closed", "2026-10-23T12:00:00Z", "never"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	_, _, out = mockMCP(t, map[string]any{"items": []any{}}, func() { runRestsCommand([]string{"--stored", "Decision:D9"}) })
	if !strings.Contains(out, "no item you can read") {
		t.Errorf("empty:\n%s", out)
	}
	_, _, out = mockMCP(t, result, func() { runRestsCommand([]string{"--stored", "--json", "Decision:D1"}) })
	if !strings.Contains(out, `"items"`) {
		t.Errorf("--json:\n%s", out)
	}
}

func TestPremise_Confirm(t *testing.T) {
	tool, args, out := mockMCP(t, map[string]any{"confirmed": true}, func() {
		runPremiseCommand([]string{"confirm", "D12", "D7", "--reason", "still right"})
	})
	if tool != "confirm_premise" || args["decision"] != "D12" || args["premise"] != "D7" || args["reason"] != "still right" {
		t.Errorf("tool %q, args %v", tool, args)
	}
	if !strings.Contains(out, "confirmed: D12 still holds although D7 is not in force") {
		t.Errorf("out:\n%s", out)
	}
	_, args, _ = mockMCP(t, map[string]any{"confirmed": true}, func() {
		runPremiseCommand([]string{"confirm", "--reason=why", "D12", "D7"})
	})
	if args["reason"] != "why" {
		t.Errorf("--reason=: %v", args)
	}
	_, args, out = mockMCP(t, map[string]any{"confirmed": true}, func() {
		runPremiseCommand([]string{"confirm", "S3170", "--reason", "still right"})
	})
	if args["condition"] != "S3170" || args["decision"] != nil || !strings.Contains(out, "the condition S3170 is resolved") {
		t.Errorf("by the condition: %v\n%s", args, out)
	}
	_, _, out = mockMCP(t, nil, func() { runPremiseCommand([]string{"--help"}) })
	if !strings.Contains(out, "premise confirm") {
		t.Errorf("--help:\n%s", out)
	}
}
