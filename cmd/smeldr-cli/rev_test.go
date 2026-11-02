package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// revCall is one MCP call the fake server saw.
type revCall struct {
	tool string
	args map[string]any
}

// revServer is a fake MCP server that answers by tool: a result map, or an
// error code and message. It records every call.
type revServer struct {
	results map[string]any
	errors  map[string][2]any // tool -> {code, message}
	calls   []revCall
}

func (s *revServer) start(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		_ = json.Unmarshal(body, &req)
		s.calls = append(s.calls, revCall{req.Params.Name, req.Params.Arguments})
		if e, ok := s.errors[req.Params.Name]; ok {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "error": map[string]any{"code": e[0], "message": e[1]}})
			return
		}
		data, _ := json.Marshal(s.results[req.Params.Name])
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1,
			"result": map[string]any{"content": []map[string]any{{"type": "text", "text": string(data)}}}})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SMELDR_URL", srv.URL)
	t.Setenv("SMELDR_TOKEN", "test-token")
	t.Setenv("SMELDR_MCP_URL", srv.URL)
}

// tools lists the tools called, in order.
func (s *revServer) tools() string {
	var names []string
	for _, c := range s.calls {
		names = append(names, c.tool)
	}
	return strings.Join(names, ",")
}

// last returns the last call to tool.
func (s *revServer) last(tool string) map[string]any {
	for i := len(s.calls) - 1; i >= 0; i-- {
		if s.calls[i].tool == tool {
			return s.calls[i].args
		}
	}
	return nil
}

// withTerminal sets whether stdin is a terminal and what the person types.
func withTerminal(t *testing.T, terminal bool, typed string) {
	t.Helper()
	oldTerm, oldIn := isTerminal, stdin
	isTerminal = func() bool { return terminal }
	stdin = strings.NewReader(typed)
	t.Cleanup(func() { isTerminal, stdin = oldTerm, oldIn })
}

var revDecision = map[string]any{"address": "D170", "title": "Use SQLite", "Status": "proposed", "Rev": float64(6), "body": "the body"}

// Without --rev on a terminal: cli shows the record, the person confirms,
// and the write names the rev they saw.
func TestRev_ConfirmThenWrite(t *testing.T) {
	cases := []struct {
		name, read, write string
		run               func()
		results           map[string]any
		want              int
	}{
		{"transition", "get_decision", "transition_item",
			func() { runTransitionCommand([]string{"Decision", "D170", "--to", "ratified"}) },
			map[string]any{"get_decision": revDecision, "transition_item": map[string]any{"status": "ratified"}}, 6},
		{"decision sources", "get_decision", "update_decision",
			func() { runDecisionSourcesCommand([]string{"D170", "--clear"}) },
			map[string]any{"get_decision": revDecision}, 6},
		{"check answer", "get_change_check", "answer_change_check",
			func() { runCheckCommand([]string{"answer", "--approve", "CHK3", "ARE1"}) },
			map[string]any{"get_change_check": map[string]any{"address": "CHK3", "rev": float64(2)}, "answer_change_check": cliCheck}, 2},
		{"premise confirm", "get_decision", "confirm_premise",
			func() { runPremiseCommand([]string{"confirm", "D170", "D7", "--reason", "still right"}) },
			map[string]any{"get_decision": revDecision}, 6},
		{"grant", "list_grants", "grant_role",
			func() { runGrantCommand([]string{"tok-1", "editor"}) },
			map[string]any{"list_grants": map[string]any{"grants": []any{map[string]any{"ID": "g1", "RoleName": "author"}}, "identity_rev": float64(3)}}, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &revServer{results: c.results}
			s.start(t)
			withTerminal(t, true, "y\n")
			code, stderr := runExpectingExit(t, func() { captureStdout(t, c.run) })
			if code != -1 {
				t.Fatalf("exit %d: %s", code, stderr)
			}
			if !strings.Contains(stderr, "Proceed with rev") || s.calls[0].tool != c.read {
				t.Errorf("shown and asked first: calls %s\n%s", s.tools(), stderr)
			}
			if got := fmt.Sprint(s.last(c.write)["rev"]); got != fmt.Sprint(c.want) {
				t.Errorf("%s rev = %s; want %d", c.write, got, c.want)
			}
		})
	}
}

// Declined: nothing written. No terminal: refused before any write.
func TestRev_DeclinedAndNoTerminal(t *testing.T) {
	s := &revServer{results: map[string]any{"get_decision": revDecision}}
	s.start(t)
	withTerminal(t, true, "n\n")
	code, stderr := runExpectingExit(t, func() { runTransitionCommand([]string{"Decision", "D170", "--to", "ratified"}) })
	if code != 1 || s.last("transition_item") != nil || !strings.Contains(stderr, "nothing written") {
		t.Errorf("declined: exit %d, calls %s\n%s", code, s.tools(), stderr)
	}
	s.calls = nil
	withTerminal(t, false, "")
	code, stderr = runExpectingExit(t, func() { runTransitionCommand([]string{"Decision", "D170", "--to", "ratified"}) })
	if code != 1 || len(s.calls) != 0 || !strings.Contains(stderr, "needs --rev") {
		t.Errorf("no terminal: exit %d, calls %s\n%s", code, s.tools(), stderr)
	}
}

// --rev is sent as given, with no read and no prompt; other types name none.
func TestRev_FlagSentAsIs(t *testing.T) {
	s := &revServer{results: map[string]any{"transition_item": map[string]any{}}}
	s.start(t)
	withTerminal(t, false, "")
	code, stderr := runExpectingExit(t, func() {
		captureStdout(t, func() { runTransitionCommand([]string{"Decision", "D170", "--to", "ratified", "--rev=9"}) })
	})
	if code != -1 || s.tools() != "transition_item" || fmt.Sprint(s.last("transition_item")["rev"]) != "9" {
		t.Errorf("--rev: exit %d, calls %s %v\n%s", code, s.tools(), s.calls, stderr)
	}
	s.calls = nil
	runExpectingExit(t, func() { captureStdout(t, func() { runTransitionCommand([]string{"Task", "T1", "--to", "done"}) }) })
	if s.tools() != "transition_item" || s.last("transition_item")["rev"] != nil {
		t.Errorf("a Task names no rev: %s %v", s.tools(), s.calls)
	}
	s.calls = nil
	runExpectingExit(t, func() {
		captureStdout(t, func() { runTransitionCommand([]string{"Task", "T1", "--to", "done", "--rev", "1"}) })
	})
	if fmt.Sprint(s.last("transition_item")["rev"]) != "1" {
		t.Errorf("--rev on another type is sent: %v", s.calls)
	}
	for _, bad := range [][]string{{"Decision", "D1", "--to", "x", "--rev"}, {"Decision", "D1", "--to", "x", "--rev", "-1"}, {"Decision", "D1", "--to", "x", "--rev=x"}} {
		if code, _ := runExpectingExit(t, func() { runTransitionCommand(bad) }); code != 1 {
			t.Errorf("%v: exit %d; want 1", bad, code)
		}
	}
}

// A rev_conflict prints core's message and exits 1, never a retry; a stale
// ratify also shows the Decision as it is now.
func TestRev_ConflictExits(t *testing.T) {
	msg := "Decision D170 changed since you read it (you sent rev 4, it is now rev 6)."
	s := &revServer{results: map[string]any{"get_decision": revDecision}, errors: map[string][2]any{"transition_item": {-32002, msg}}}
	s.start(t)
	withTerminal(t, false, "")
	code, stderr := runExpectingExit(t, func() { runTransitionCommand([]string{"Decision", "D170", "--to", "ratified", "--rev", "4"}) })
	if code != 1 || !strings.Contains(stderr, msg) || !strings.Contains(stderr, "It is now") || !strings.Contains(stderr, "the body") {
		t.Errorf("stale ratify: exit %d\n%s", code, stderr)
	}
	n := 0
	for _, c := range s.calls {
		if c.tool == "transition_item" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d writes; want exactly one, never a retry", n)
	}
	s.errors = map[string][2]any{"transition_item": {-32602, "invalid"}}
	if code, stderr := runExpectingExit(t, func() { runTransitionCommand([]string{"Decision", "D170", "--to", "ratified", "--rev", "4"}) }); code != 1 || !strings.Contains(stderr, "invalid") {
		t.Errorf("another error: exit %d\n%s", code, stderr)
	}
}

// Warnings in a result go to stderr.
func TestRev_WarningsPrinted(t *testing.T) {
	s := &revServer{results: map[string]any{"transition_item": map[string]any{"warnings": []any{map[string]any{"code": "rev_missing", "message": "send the rev"}}}}}
	s.start(t)
	_, stderr := runExpectingExit(t, func() {
		captureStdout(t, func() { runTransitionCommand([]string{"Task", "T1", "--to", "done"}) })
	})
	if !strings.Contains(stderr, "warning: rev_missing: send the rev") {
		t.Errorf("warnings:\n%s", stderr)
	}
}

// premise confirm by condition and relation withdraw need --rev; with it,
// it is sent.
func TestRev_FlagRequired(t *testing.T) {
	s := &revServer{results: map[string]any{}}
	s.start(t)
	withTerminal(t, true, "y\n")
	for name, run := range map[string]func(){
		"premise by condition": func() { runPremiseCommand([]string{"confirm", "S3170", "--reason", "r"}) },
		"relation withdraw":    func() { runRelationCommand([]string{"withdraw", "rel-1", "--reason", "r"}) },
	} {
		code, stderr := runExpectingExit(t, run)
		if code != 1 || len(s.calls) != 0 || !strings.Contains(stderr, "needs --rev") {
			t.Errorf("%s without --rev: exit %d, calls %s\n%s", name, code, s.tools(), stderr)
		}
	}
	code, stderr := runExpectingExit(t, func() {
		captureStdout(t, func() { runRelationCommand([]string{"withdraw", "rel-1", "--reason", "r", "--rev", "3"}) })
	})
	if code != -1 || fmt.Sprint(s.last("withdraw_relation")["rev"]) != "3" {
		t.Errorf("withdraw with --rev: exit %d %v\n%s", code, s.calls, stderr)
	}
}

// Grants: a holder that is not an identity names no rev; a revoke finds the
// holder first; an unknown grant goes to the server as before.
func TestRev_Grants(t *testing.T) {
	s := &revServer{results: map[string]any{"list_grants": map[string]any{"grants": []any{}}, "grant_role": map[string]any{"id": "g9"}}}
	s.start(t)
	withTerminal(t, false, "")
	code, stderr := runExpectingExit(t, func() { captureStdout(t, func() { runGrantCommand([]string{"tok-1", "editor"}) }) })
	if code != -1 || s.last("grant_role")["rev"] != nil {
		t.Errorf("no identity: exit %d %v\n%s", code, s.calls, stderr)
	}
	s.results["list_grants"] = map[string]any{"grants": []any{map[string]any{"ID": "g1", "TokenID": "tok-1", "RoleName": "editor"}}, "identity_rev": float64(4)}
	s.results["revoke_grant"] = map[string]any{"revoked": true}
	s.calls = nil
	withTerminal(t, true, "yes\n")
	code, stderr = runExpectingExit(t, func() { captureStdout(t, func() { runGrantCommand([]string{"revoke", "g1"}) }) })
	if code != -1 || fmt.Sprint(s.last("revoke_grant")["rev"]) != "4" || s.calls[1].args["token_id"] != "tok-1" {
		t.Errorf("revoke: exit %d %v\n%s", code, s.calls, stderr)
	}
	s.calls = nil
	runExpectingExit(t, func() { captureStdout(t, func() { runGrantCommand([]string{"revoke", "g-unknown"}) }) })
	if s.last("revoke_grant")["rev"] != nil {
		t.Errorf("an unknown grant names no rev: %v", s.calls)
	}
	withTerminal(t, false, "")
	code, stderr = runExpectingExit(t, func() { runGrantCommand([]string{"tok-1", "editor"}) })
	if code != 1 || !strings.Contains(stderr, "needs --rev") {
		t.Errorf("an identity without a terminal: exit %d\n%s", code, stderr)
	}
	if code, _ := runExpectingExit(t, func() { runGrantCommand([]string{"list", "--rev", "1"}) }); code != 1 {
		t.Errorf("grant list --rev: exit %d; want 1", code)
	}
}

// The read failing, or returning no rev, writes nothing.
func TestRev_ReadFails(t *testing.T) {
	s := &revServer{results: map[string]any{"get_decision": map[string]any{"address": "D1"}}, errors: map[string][2]any{"get_change_check": {-32602, "no such check"}}}
	s.start(t)
	withTerminal(t, true, "y\n")
	code, stderr := runExpectingExit(t, func() { runTransitionCommand([]string{"Decision", "D1", "--to", "ratified"}) })
	if code != 1 || s.last("transition_item") != nil || !strings.Contains(stderr, "no rev") {
		t.Errorf("no rev in the read: exit %d\n%s", code, stderr)
	}
	code, _ = runExpectingExit(t, func() { runCheckCommand([]string{"answer", "--approve", "CHK3", "ARE1"}) })
	if code != 1 || s.last("answer_change_check") != nil {
		t.Errorf("a failed read: exit %d, calls %s", code, s.tools())
	}
}

// init's founder grants name the rev each previous grant returned.
func TestInit_GrantsNameRev(t *testing.T) {
	s := &revServer{results: map[string]any{
		"list_grants": map[string]any{"grants": []any{}, "identity_rev": float64(0)},
		"grant_role":  map[string]any{"id": "g", "identity_rev": float64(1)},
	}}
	s.start(t)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	rev := initIdentityRev(cfg, "tok-1")
	if rev == nil || *rev != 0 {
		t.Fatalf("initIdentityRev = %v", rev)
	}
	if next := nextIdentityRev(`{"identity_rev": 2}`); next == nil || *next != 2 {
		t.Errorf("nextIdentityRev = %v", next)
	}
	if next := nextIdentityRev(`not json`); next != nil {
		t.Errorf("nextIdentityRev of junk = %v", next)
	}
	s.errors = map[string][2]any{"list_grants": {-32603, "down"}}
	if rev := initIdentityRev(cfg, "tok-1"); rev != nil {
		t.Errorf("a failed read: %v", rev)
	}
}

func TestRev_Helpers(t *testing.T) {
	if n, ok := revOf(`{"rev": 3}`); !ok || n != 3 {
		t.Errorf("revOf rev = %d %v", n, ok)
	}
	if _, ok := revOf(`nope`); ok {
		t.Error("revOf of junk")
	}
	if got := describeRecord(`{"slug": "s", "name": "N"}`, 1, false); got != `s "N", rev 1` {
		t.Errorf("describeRecord = %q", got)
	}
	if got := describeRecord(`junk`, 2, false); got != "rev 2" {
		t.Errorf("describeRecord of junk = %q", got)
	}
	if revGetTool("CoverageRule") != "get_coverage_rule" || revGetTool("Task") != "" {
		t.Error("revGetTool")
	}
	if !strings.Contains(revTypes(), "coverage_rule") {
		t.Errorf("revTypes = %s", revTypes())
	}
}
