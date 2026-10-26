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

// TestCheckFlagOrder (T958): every check subcommand reads its flags before,
// after or among its arguments, and refuses an unknown flag or a stray
// argument instead of ignoring it.
func TestCheckFlagOrder(t *testing.T) {
	repo := []string{"--repo", "acme/app", "--number", "7", "--head", cliHead}
	cat := func(parts ...[]string) []string {
		var out []string
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	for _, tc := range []struct {
		name    string
		parse   func([]string) (checkCall, error)
		args    []string
		want    map[string]any
		raw     bool
		wantErr string
	}{
		{"change, flags before", parseCheckChange, cat(repo, []string{"a.go", "b.go"}),
			map[string]any{"paths": "[a.go b.go]", "number": "7"}, false, ""},
		{"change, flags after", parseCheckChange, cat([]string{"a.go", "b.go"}, repo, []string{"--files-total", "9"}),
			map[string]any{"paths": "[a.go b.go]", "files_total": "9"}, false, ""},
		{"change, mixed", parseCheckChange, cat([]string{"a.go", "--json"}, repo, []string{"b.go"}),
			map[string]any{"paths": "[a.go b.go]", "repository": "acme/app"}, true, ""},
		{"change, unknown flag", parseCheckChange, cat(repo, []string{"a.go", "--file-total", "9"}), nil, false, "-file-total"},
		{"answer, flags after", parseCheckAnswer, []string{"CHK1", "ARE1", "--approve"},
			map[string]any{"check": "CHK1", "area": "ARE1", "approve": "true"}, false, ""},
		{"answer, flags before", parseCheckAnswer, []string{"--approve", "CHK1", "ARE1"},
			map[string]any{"check": "CHK1", "approve": "true"}, false, ""},
		{"answer, mixed", parseCheckAnswer, []string{"CHK1", "--not-approved", "ARE1", "--reason", "r"},
			map[string]any{"area": "ARE1", "approve": "false", "reason": "r"}, false, ""},
		{"answer, a third argument", parseCheckAnswer, []string{"CHK1", "ARE1", "ARE2", "--approve"}, nil, false, "needs --approve or --not-approved"},
		{"answer, both", parseCheckAnswer, []string{"CHK1", "ARE1", "--approve", "--not-approved"}, nil, false, "needs --approve or --not-approved"},
		{"answer, neither", parseCheckAnswer, []string{"CHK1", "ARE1"}, nil, false, "needs --approve or --not-approved"},
		{"answer, misspelled", parseCheckAnswer, []string{"CHK1", "ARE1", "--aprove"}, nil, false, "-aprove"},
		{"merge, flags after a stray", parseCheckMerge, cat([]string{"x"}, repo, []string{"--merge", cliHead}), nil, false, `check merge: unexpected argument "x"`},
		{"merge", parseCheckMerge, cat(repo, []string{"--merge", cliHead, "--json"}), map[string]any{"merge_commit": cliHead}, true, ""},
		{"list, flags", parseCheckList, []string{"--repo", "acme/app", "--number", "2"}, map[string]any{"number": "2"}, false, ""},
		{"list, a stray", parseCheckList, []string{"--repo", "acme/app", "extra"}, nil, false, `check list: unexpected argument "extra"`},
		{"show, flag before", parseCheckShow, []string{"--json", "CHK1"}, map[string]any{"check": "CHK1"}, true, ""},
		{"show, two checks", parseCheckShow, []string{"CHK1", "CHK2"}, nil, false, "check show <check>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := tc.parse(tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range tc.want {
				if got := fmt.Sprint(c.params[k]); got != v {
					t.Errorf("%s = %s, want %s (params %v)", k, got, v, c.params)
				}
			}
			if c.raw != tc.raw {
				t.Errorf("raw = %v", c.raw)
			}
		})
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
