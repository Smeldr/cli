package main

import (
	"strings"
	"testing"
)

func TestStanding_ToolAndArgs(t *testing.T) {
	tests := []struct {
		name   string
		result map[string]any
		want   string
	}{
		{"holds", map[string]any{"type_name": "Decision", "slug": "dec-1", "standing": "holds"}, "holds"},
		{"ceased", map[string]any{"type_name": "Decision", "slug": "dec-1", "standing": "ceased"}, "ceased"},
		{"none", map[string]any{"type_name": "Decision", "slug": "dec-1", "standing": "none"}, "none"},
		{"no standing for the type", map[string]any{"type_name": "Decision", "slug": "dec-1"}, "no standing for this type"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool, args, out := mockMCP(t, tc.result, func() {
				runStandingCommand([]string{"Decision", "dec-1"})
			})
			if tool != "get_item_standing" {
				t.Errorf("tool = %q, want get_item_standing", tool)
			}
			if args["type_name"] != "Decision" || args["slug"] != "dec-1" {
				t.Errorf("args = %v", args)
			}
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("output = %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

func TestStanding_JSONFlag(t *testing.T) {
	_, _, out := mockMCP(t, map[string]any{"type_name": "Decision", "slug": "dec-1", "standing": "holds"}, func() {
		runStandingCommand([]string{"Decision", "dec-1", "--json"})
	})
	if !strings.Contains(out, `"standing"`) || !strings.Contains(out, `"holds"`) {
		t.Errorf("--json should print the whole result:\n%s", out)
	}
}

func TestStanding_Help(t *testing.T) {
	out := captureStdout(t, func() { runStandingCommand([]string{"--help"}) })
	for _, want := range []string{"smeldr-cli standing", "holds, ceased or none", "could not read the type's flow", "get\" and \"<type> list\""} {
		if !strings.Contains(out, want) {
			t.Errorf("help is missing %q:\n%s", want, out)
		}
	}
}
