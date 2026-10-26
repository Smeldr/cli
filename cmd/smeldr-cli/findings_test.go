package main

import (
	"strings"
	"testing"
)

func TestFindings_ToolAndOutput(t *testing.T) {
	result := map[string]any{"findings": []any{
		map[string]any{"detector": "standing-drift", "subject_type": "Decision", "subject_id": "d1", "address": "D12", "message": "item drifted", "last_seen_at": "2026-10-26T17:00:00Z"},
		map[string]any{"detector": "structural", "subject_type": "RelationEdge", "subject_id": "e9", "message": "relation target no longer alive", "last_seen_at": "2026-10-26T16:00:00Z"},
	}}
	tool, args, out := mockMCP(t, result, func() {
		runFindingsCommand([]string{"--detector", "standing-drift", "--type", "Decision", "--limit", "5"})
	})
	if tool != "list_findings" || args["detector"] != "standing-drift" || args["subject_type"] != "Decision" || args["limit"] != float64(5) {
		t.Errorf("tool %q, args %v", tool, args)
	}
	for _, want := range []string{"DETECTOR", "SUBJECT", "D12", "item drifted", "RelationEdge:e9", "2026-10-26 17:00"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	_, _, out = mockMCP(t, map[string]any{"findings": []any{}}, func() { runFindingsCommand(nil) })
	if !strings.Contains(out, "no findings") {
		t.Errorf("empty:\n%s", out)
	}
	_, _, out = mockMCP(t, result, func() { runFindingsCommand([]string{"--json"}) })
	if !strings.Contains(out, `"findings"`) {
		t.Errorf("--json:\n%s", out)
	}
	_, _, out = mockMCP(t, result, func() { runFindingsCommand([]string{"--help"}) })
	if !strings.Contains(out, "what the detectors found") {
		t.Errorf("--help:\n%s", out)
	}
}
