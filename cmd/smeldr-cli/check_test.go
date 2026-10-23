package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const cliHead = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

var cliCheck = map[string]any{
	"covered": true,
	"check": map[string]any{"address": "CHK3", "Status": "awaiting-approval", "repository": "acme/app", "number": 7,
		"head_commit": cliHead, "files_total": 5000, "files_checked": 3000, "checked_partly": true, "setting": "requires-approval"},
	"areas":     []any{map[string]any{"address": "ARE1", "name": "Billing", "approval_mode": "requires-approval", "verdict": "awaiting-approval"}},
	"decisions": []any{map[string]any{"address": "D7", "title": "Money in cents"}},
}

func TestCheckChange_ToolAndArgs(t *testing.T) {
	stdin = strings.NewReader("cmd/x/main.go\n\n")
	old := stdin
	t.Cleanup(func() { stdin = old })
	tool, args, out := mockMCP(t, cliCheck, func() {
		runCheckCommand([]string{"change", "--repo", "acme/app", "--number", "7", "--head", cliHead, "--files-total", "5000", "--stdin", "internal/billing/a.go"})
	})
	if tool != "check_change" || args["repository"] != "acme/app" || fmt.Sprint(args["paths"]) != "[internal/billing/a.go cmd/x/main.go]" || fmt.Sprint(args["files_total"]) != "5000" {
		t.Errorf("tool %q, args %v", tool, args)
	}
	for _, want := range []string{"CHK3", "awaiting-approval", "acme/app#7", "3000/5000", "checked partly", "ARE1", "Billing", "D7", "Money in cents"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	_, _, out = mockMCP(t, map[string]any{"covered": false, "uncovered": []any{"a"}}, func() {
		runCheckCommand([]string{"change", "--repo", "acme/app", "--number", "7", "--head", cliHead, "a"})
	})
	if !strings.Contains(out, "no Check") {
		t.Errorf("uncovered:\n%s", out)
	}
	_, _, out = mockMCP(t, cliCheck, func() {
		runCheckCommand([]string{"change", "--repo", "acme/app", "--number", "7", "--head", cliHead, "--json", "a"})
	})
	if !strings.Contains(out, `"covered"`) {
		t.Errorf("--json:\n%s", out)
	}
}

func TestCheckMergeAnswerShowList(t *testing.T) {
	tool, args, out := mockMCP(t, map[string]any{"covered": true, "head_mismatch": true, "opened": []any{"ARE1"}}, func() {
		runCheckCommand([]string{"merge", "--repo", "acme/app", "--number", "7", "--head", cliHead, "--merge", cliHead})
	})
	if tool != "record_merge" || args["merge_commit"] != cliHead || !strings.Contains(out, "audit opened for [ARE1]") || !strings.Contains(out, "not the checked head") {
		t.Errorf("merge: %q %v\n%s", tool, args, out)
	}
	for result, want := range map[string]string{`{"covered": false}`: "nothing to audit", `{"covered": true, "opened": []}`: "no audit opened"} {
		var m map[string]any
		_ = json.Unmarshal([]byte(result), &m)
		_, _, out = mockMCP(t, m, func() {
			runCheckCommand([]string{"merge", "--repo", "acme/app", "--number", "7", "--head", cliHead, "--merge", cliHead})
		})
		if !strings.Contains(out, want) {
			t.Errorf("merge %s:\n%s", result, out)
		}
	}
	_, _, out = mockMCP(t, map[string]any{"opened": []any{}}, func() {
		runCheckCommand([]string{"merge", "--repo", "acme/app", "--number", "7", "--head", cliHead, "--merge", cliHead, "--json"})
	})
	if !strings.Contains(out, `"opened"`) {
		t.Errorf("merge --json:\n%s", out)
	}
	tool, args, _ = mockMCP(t, cliCheck, func() {
		runCheckCommand([]string{"answer", "--not-approved", "--reason", "breaks the rule", "CHK3", "ARE1"})
	})
	if tool != "answer_change_check" || args["approve"] != false || args["reason"] != "breaks the rule" || args["check"] != "CHK3" || args["area"] != "ARE1" {
		t.Errorf("answer: %q %v", tool, args)
	}
	tool, args, out = mockMCP(t, cliCheck, func() { runCheckCommand([]string{"show", "CHK3"}) })
	if tool != "get_change_check" || args["check"] != "CHK3" || !strings.Contains(out, "CHK3") {
		t.Errorf("show: %q %v\n%s", tool, args, out)
	}
	_, _, out = mockMCP(t, cliCheck, func() { runCheckCommand([]string{"show", "CHK3", "--json"}) })
	if !strings.Contains(out, `"areas"`) {
		t.Errorf("show --json:\n%s", out)
	}
	list := map[string]any{"checks": []any{map[string]any{"address": "CHK3", "Status": "approved", "repository": "acme/app", "number": 7, "head_commit": cliHead}}}
	tool, args, out = mockMCP(t, list, func() { runCheckCommand([]string{"list", "--repo", "acme/app", "--number", "7"}) })
	if tool != "list_change_checks" || fmt.Sprint(args["number"]) != "7" || !strings.Contains(out, "acme/app#7") || !strings.Contains(out, "approved") {
		t.Errorf("list: %q %v\n%s", tool, args, out)
	}
	_, _, out = mockMCP(t, map[string]any{"checks": []any{}}, func() { runCheckCommand([]string{"list", "--area", "ARE1"}) })
	if !strings.Contains(out, "no Checks") {
		t.Errorf("empty list:\n%s", out)
	}
	_, _, out = mockMCP(t, list, func() { runCheckCommand([]string{"list", "--area", "ARE1", "--json"}) })
	if !strings.Contains(out, `"checks"`) {
		t.Errorf("list --json:\n%s", out)
	}
	_, _, out = mockMCP(t, list, func() { runCheckCommand([]string{"--help"}) })
	if !strings.Contains(out, "the pull request check") {
		t.Errorf("help:\n%s", out)
	}
}
