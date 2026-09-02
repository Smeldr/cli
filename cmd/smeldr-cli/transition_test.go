package main

import (
	"strings"
	"testing"
)

func TestTransition_ToolAndArgs_Ratification(t *testing.T) {
	result := map[string]any{"id": "dec-1", "slug": "dec-1", "status": "ratified"}
	tool, args, out := mockMCP(t, result, func() {
		runTransitionCommand([]string{"Decision", "dec-1", "--to", "ratified"})
	})
	if tool != "transition_item" {
		t.Errorf("tool = %q, want transition_item", tool)
	}
	if args["type_name"] != "Decision" {
		t.Errorf("type_name = %v, want Decision", args["type_name"])
	}
	if args["slug"] != "dec-1" {
		t.Errorf("slug = %v, want dec-1", args["slug"])
	}
	if args["to_state"] != "ratified" {
		t.Errorf("to_state = %v, want ratified", args["to_state"])
	}
	if _, ok := args["reason"]; ok {
		t.Errorf("reason present without --reason: %v", args["reason"])
	}
	if !strings.Contains(out, "ratified") {
		t.Errorf("output should contain the result JSON:\n%s", out)
	}
}

func TestTransition_WithReason(t *testing.T) {
	result := map[string]any{"id": "item-1", "status": "approved"}
	_, args, _ := mockMCP(t, result, func() {
		runTransitionCommand([]string{"widget", "item-1", "--to", "approved", "--reason", "looks good"})
	})
	if args["reason"] != "looks good" {
		t.Errorf("reason = %v, want %q", args["reason"], "looks good")
	}
}
