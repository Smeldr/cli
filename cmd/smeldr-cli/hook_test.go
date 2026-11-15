package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hookServer is a fake MCP server for decisions_for_paths. answer is the
// tool's text; status and body, when set, replace the whole response.
type hookServer struct {
	answer string
	status int
	body   string
	delay  time.Duration
	calls  int
	paths  []any
	repo   any
	// texts answers get_decision by the slug asked for; a slug not in it
	// gets a tool error. bodyCalls counts those calls.
	texts     map[string]string
	bodyCalls int
	bodyDelay time.Duration
}

func (h *hookServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		_ = json.Unmarshal(raw, &req)
		if req.Params.Name == "get_decision" {
			h.bodyCalls++
			if h.bodyDelay > 0 {
				time.Sleep(h.bodyDelay)
			}
			slug, _ := req.Params.Arguments["slug"].(string)
			text, ok := h.texts[slug]
			b, _ := json.Marshal(map[string]any{"body": text})
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{
				"content": []map[string]any{{"type": "text", "text": string(b)}}, "isError": !ok}})
			return
		}
		h.calls++
		h.paths, _ = req.Params.Arguments["paths"].([]any)
		h.repo = req.Params.Arguments["repository"]
		if h.delay > 0 {
			time.Sleep(h.delay)
		}
		if h.status != 0 {
			w.WriteHeader(h.status)
			fmt.Fprint(w, h.body)
			return
		}
		if h.body != "" {
			fmt.Fprint(w, h.body)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{
			"content": []map[string]any{{"type": "text", "text": h.answer}}, "isError": false}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// hookEnv points the hook at srv (or at nothing, for "") with a fresh state
// directory and no project directory from the environment.
func hookEnv(t *testing.T, url string) {
	t.Helper()
	t.Setenv("SMELDR_URL", url)
	t.Setenv("SMELDR_TOKEN", "hook-secret-token-value")
	t.Setenv("SMELDR_MCP_URL", url)
	t.Setenv("SMELDR_HOOK_STATE_DIR", t.TempDir())
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	old := http.DefaultClient.Timeout
	t.Cleanup(func() { http.DefaultClient.Timeout = old })
}

// captureStderr redirects os.Stderr for the duration of fn and returns what was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	defer func() { os.Stderr = old }()
	fn()
	w.Close()
	return <-done
}

// editCall is a PreToolUse input for an Edit of file in session.
func editCall(session, cwd, file string) string {
	b, _ := json.Marshal(map[string]any{"session_id": session, "cwd": cwd, "hook_event_name": "PreToolUse",
		"tool_name": "Edit", "tool_input": map[string]any{"file_path": file, "old_string": "a", "new_string": "b"}})
	return string(b)
}

// runHook runs the pre-edit hook and decodes its one line of output.
func runHook(t *testing.T, args []string, input string) (preToolUseOutput, string) {
	t.Helper()
	var out bytes.Buffer
	runPreEditHook(args, strings.NewReader(input), &out)
	return decodeHookOutput(t, out.String()), out.String()
}

func decodeHookOutput(t *testing.T, raw string) preToolUseOutput {
	t.Helper()
	var o preToolUseOutput
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) != 1 || json.Unmarshal([]byte(lines[0]), &o) != nil {
		t.Fatalf("want one JSON line, got %q", raw)
	}
	if o.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Errorf("hookEventName = %q", o.HookSpecificOutput.HookEventName)
	}
	return o
}

const coveredAnswer = `{"repository":"acme/app","areas":[{"address":"ARE1","name":"Billing","approval_mode":"requires-approval","paths":["internal/billing/invoice.go"]}],
"decisions":[{"address":"D1","title":"Log no session tokens","place":"organization","place_address":"ORG1"},{"address":"D3","title":"Invoices are immutable","place":"area","place_address":"ARE1"}],"uncovered":[]}`

func TestPreEditHook_CoveredDeniesOnceThenAllows(t *testing.T) {
	h := &hookServer{answer: coveredAnswer}
	hookEnv(t, h.start(t).URL)
	root := t.TempDir()
	file := filepath.Join(root, "internal", "billing", "invoice.go")
	args := []string{"--repo", "acme/app", "--root", root}

	o, _ := runHook(t, args, editCall("s1", root, file))
	if o.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("first edit: %+v", o)
	}
	reason := o.HookSpecificOutput.PermissionDecisionReason
	for _, want := range []string{"internal/billing/invoice.go", "acme/app", "the Smeldr instance at http://", "Area ARE1 Billing (approval: requires-approval)",
		"Decision D1 (organization ORG1): Log no session tokens", "Decision D3 (area ARE1): Invoices are immutable",
		"make the same edit again now: this hook lets it through", "stop and ask the user", "does not check the change"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason is missing %q:\n%s", want, reason)
		}
	}
	if h.repo != "acme/app" || fmt.Sprint(h.paths) != "[internal/billing/invoice.go]" {
		t.Errorf("asked repo %v paths %v", h.repo, h.paths)
	}

	if o, _ := runHook(t, args, editCall("s1", root, file)); o.HookSpecificOutput.PermissionDecision != "allow" {
		t.Errorf("second edit: %+v", o)
	}
	// Another session has not been shown it.
	if o, _ := runHook(t, args, editCall("s2", root, file)); o.HookSpecificOutput.PermissionDecision != "deny" {
		t.Errorf("other session: %+v", o)
	}
	// A Decision ratified mid-session changes the set: shown again.
	h.answer = strings.Replace(coveredAnswer, `"uncovered"`, `"extra":1,"uncovered"`, 1)
	h.answer = strings.Replace(h.answer, `]`+",\n"+`"decisions":[`, `],"decisions":[{"address":"D9","title":"New","place":"area","place_address":"ARE1"},`, 1)
	if !strings.Contains(h.answer, "D9") {
		t.Fatal("test setup: D9 not added")
	}
	if o, _ := runHook(t, args, editCall("s1", root, file)); o.HookSpecificOutput.PermissionDecision != "deny" ||
		!strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, "D9") {
		t.Errorf("changed set: %+v", o)
	}
}

// The deny carries each Decision's text, so an agent without this instance's
// tools has read them when it makes the edit again (found in the T1207
// session test: told to "read them (get_decision)", the agent had no such
// tool and stopped).
func TestPreEditHook_DenyShowsTheDecisionsText(t *testing.T) {
	long := strings.Repeat("word ", 200)
	answer := `{"areas":[{"address":"ARE1","name":"Billing","approval_mode":"comment-only"}],"decisions":[
{"id":"id-1","address":"D1","title":"Log no session tokens","place":"organization","place_address":"ORG1"},
{"id":"id-2","address":"D2","title":"Invoices are immutable","place":"area","place_address":"ARE1"},
{"id":"id-3","address":"D3","title":"Long","place":"area","place_address":"ARE1"}],"uncovered":[]}`
	h := &hookServer{answer: answer, texts: map[string]string{"id-1": "Session tokens never\n\nreach a log line.", "id-3": long}}
	hookEnv(t, h.start(t).URL)
	root := t.TempDir()
	args := []string{"--repo", "acme/app", "--root", root}
	call := editCall("s", root, filepath.Join(root, "a.go"))
	o, _ := runHook(t, args, call)
	reason := o.HookSpecificOutput.PermissionDecisionReason
	if !strings.Contains(reason, "Decision D1 (organization ORG1): Log no session tokens\n  Text: Session tokens never reach a log line.\n") {
		t.Errorf("D1's text is not on one line under it:\n%s", reason)
	}
	// D2's text cannot be read: its title, and a line that says so.
	if !strings.Contains(reason, "Decision D2 (area ARE1): Invoices are immutable\n  (text not readable with this token: the title is not the whole rule)\nDecision D3") {
		t.Errorf("D2 without text:\n%s", reason)
	}
	if want := "  Text: " + strings.TrimSpace(long)[:hookBodyRunes] + "...\n"; !strings.Contains(reason, want) {
		t.Errorf("D3's text is not cut to %d runes:\n%s", hookBodyRunes, reason)
	}
	if h.bodyCalls != 3 {
		t.Errorf("get_decision calls = %d, want 3", h.bodyCalls)
	}
	// The allow that follows reads no text.
	if o, _ := runHook(t, args, call); o.HookSpecificOutput.PermissionDecision != "allow" || h.bodyCalls != 3 {
		t.Errorf("second edit: %+v, get_decision calls %d", o, h.bodyCalls)
	}
}

func TestPreEditHook_TextForTheFirstTenOnly(t *testing.T) {
	var ds []string
	texts := map[string]string{}
	for i := 1; i <= hookBodyDecisions+2; i++ {
		ds = append(ds, fmt.Sprintf(`{"id":"id-%d","address":"D%d","title":"T%d","place":"organization","place_address":"ORG1"}`, i, i, i))
		texts[fmt.Sprintf("id-%d", i)] = fmt.Sprintf("text %d", i)
	}
	h := &hookServer{answer: `{"areas":[],"decisions":[` + strings.Join(ds, ",") + `],"uncovered":[]}`, texts: texts}
	hookEnv(t, h.start(t).URL)
	root := t.TempDir()
	o, _ := runHook(t, []string{"--repo", "acme/app", "--root", root}, editCall("s", root, filepath.Join(root, "a.go")))
	reason := o.HookSpecificOutput.PermissionDecisionReason
	if h.bodyCalls != hookBodyDecisions || !strings.Contains(reason, "Text: text 10\n") || strings.Contains(reason, "Text: text 11") ||
		!strings.Contains(reason, "Decision D12 (organization ORG1): T12") || !strings.Contains(reason, "for the first 10 Decisions only") {
		t.Errorf("get_decision calls %d, reason:\n%s", h.bodyCalls, reason)
	}
}

// "No Area" is allowed only when the answer lists the path as uncovered; any
// other answer with no Area and no Decision is one the hook cannot read, and
// fails closed (architect's review of the first commit).
func TestPreEditHook_UnknownAnswerShapeDenies(t *testing.T) {
	for name, answer := range map[string]string{
		"an empty object":            `{}`,
		"other keys only":            `{"paths":{"a.go":[]},"result":"ok"}`,
		"another path is uncovered":  `{"areas":[],"decisions":[],"uncovered":["b.go"]}`,
		"areas renamed by a version": `{"zones":[{"address":"ARE1"}],"decisions":[],"uncovered":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			h := &hookServer{answer: answer}
			hookEnv(t, h.start(t).URL)
			root := t.TempDir()
			o, _ := runHook(t, []string{"--repo", "acme/app", "--root", root}, editCall("s", root, filepath.Join(root, "a.go")))
			if o.HookSpecificOutput.PermissionDecision != "deny" || !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, "an answer the hook cannot read") {
				t.Errorf("%+v", o)
			}
		})
	}
}

// --timeout is the budget of the whole run: a server that is slow on
// get_decision gets the titles, a line per text not read, and the run ends
// within the budget (architect's review of the first commit).
func TestPreEditHook_OneDeadlineForTheRun(t *testing.T) {
	var ds []string
	texts := map[string]string{}
	for i := 1; i <= 5; i++ {
		ds = append(ds, fmt.Sprintf(`{"id":"id-%d","address":"D%d","title":"T%d","place":"organization","place_address":"ORG1"}`, i, i, i))
		texts[fmt.Sprintf("id-%d", i)] = fmt.Sprintf("text %d", i)
	}
	h := &hookServer{answer: `{"areas":[],"decisions":[` + strings.Join(ds, ",") + `],"uncovered":[]}`, texts: texts, bodyDelay: 150 * time.Millisecond}
	hookEnv(t, h.start(t).URL)
	root := t.TempDir()
	start := time.Now()
	o, _ := runHook(t, []string{"--repo", "acme/app", "--root", root, "--timeout", "400ms"}, editCall("s", root, filepath.Join(root, "a.go")))
	took := time.Since(start)
	reason := o.HookSpecificOutput.PermissionDecisionReason
	if o.HookSpecificOutput.PermissionDecision != "deny" || !strings.Contains(reason, "Text: text 1\n") ||
		!strings.Contains(reason, "Decision D5 (organization ORG1): T5\n  (text not read: the hook's time ran out)") ||
		!strings.Contains(reason, "Not every text could be read within the hook's --timeout") {
		t.Errorf("reason:\n%s", reason)
	}
	// 400 ms of budget, and at most one request cut off at its end: never the
	// 5 x 150 ms the texts would take one by one.
	if took > 700*time.Millisecond {
		t.Errorf("the run took %v with a 400ms budget", took)
	}
}

// The path is marked as shown only once the deny is written: a deny that
// could not be written leaves the next edit denied again.
func TestPreEditHook_StateWrittenAfterTheDeny(t *testing.T) {
	h := &hookServer{answer: coveredAnswer}
	hookEnv(t, h.start(t).URL)
	root := t.TempDir()
	args := []string{"--repo", "acme/app", "--root", root}
	call := editCall("s", root, filepath.Join(root, "a.go"))
	runPreEditHook(args, strings.NewReader(call), failWriter{})
	if o, _ := runHook(t, args, call); o.HookSpecificOutput.PermissionDecision != "deny" {
		t.Errorf("after a deny that was not written: %+v", o)
	}
	if o, _ := runHook(t, args, call); o.HookSpecificOutput.PermissionDecision != "allow" {
		t.Errorf("after a written deny: %+v", o)
	}
}

func TestPreEditHook_NewAreaShowsAgain(t *testing.T) {
	h := &hookServer{answer: `{"areas":[{"address":"ARE1","name":"Billing","approval_mode":"comment-only"}],"decisions":[],"uncovered":[]}`}
	hookEnv(t, h.start(t).URL)
	root := t.TempDir()
	args := []string{"--repo", "acme/app", "--root", root}
	call := editCall("s", root, filepath.Join(root, "a.go"))
	o, _ := runHook(t, args, call)
	if o.HookSpecificOutput.PermissionDecision != "deny" || !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, "No Decision is in force") {
		t.Fatalf("an Area with no Decision: %+v", o)
	}
	h.answer = `{"areas":[{"address":"ARE1","name":"Billing","approval_mode":"comment-only"},{"address":"ARE2","name":"Ledger","approval_mode":"comment-only"}],"decisions":[],"uncovered":[]}`
	if o, _ := runHook(t, args, call); o.HookSpecificOutput.PermissionDecision != "deny" {
		t.Errorf("a new Area: %+v", o)
	}
}

func TestPreEditHook_UncoveredAllowsAndSaysWhatTheTokenSaw(t *testing.T) {
	h := &hookServer{answer: `{"repository":"acme/app","areas":[],"decisions":[],"uncovered":["docs/readme.md"]}`}
	hookEnv(t, h.start(t).URL)
	root := t.TempDir()
	o, _ := runHook(t, []string{"--repo", "acme/app", "--root", root}, editCall("s", root, filepath.Join(root, "docs", "readme.md")))
	ctx := o.HookSpecificOutput.AdditionalContext
	if o.HookSpecificOutput.PermissionDecision != "allow" || !strings.Contains(ctx, "named no Area for docs/readme.md in acme/app to this token") ||
		!strings.Contains(ctx, "may not read is left out") || strings.Contains(ctx, "no Decision applies") {
		t.Errorf("uncovered: %+v", o)
	}
}

func TestPreEditHook_Failures(t *testing.T) {
	root := t.TempDir()
	call := editCall("s", root, filepath.Join(root, "a.go"))
	const secret = "hook-secret-token-value"
	cases := []struct {
		name  string
		setup func(t *testing.T) []string
		want  string
	}{
		{"server unreachable", func(t *testing.T) []string {
			srv := httptest.NewServer(http.NotFoundHandler())
			srv.Close()
			hookEnv(t, srv.URL)
			return nil
		}, "did not answer"},
		{"no config", func(t *testing.T) []string { hookEnv(t, ""); return nil }, "has no server to ask"},
		{"a refusal", func(t *testing.T) []string {
			h := &hookServer{body: `{"jsonrpc":"2.0","id":1,"error":{"code":-32001,"message":"forbidden"}}`}
			hookEnv(t, h.start(t).URL)
			return nil
		}, "MCP error -32001: forbidden"},
		{"a 401 that echoes the token", func(t *testing.T) []string {
			h := &hookServer{status: 401, body: "Unauthorized: Bearer " + secret}
			hookEnv(t, h.start(t).URL)
			return nil
		}, "Unauthorized: Bearer <token>"},
		{"an answer the hook cannot read", func(t *testing.T) []string {
			h := &hookServer{answer: "not json"}
			hookEnv(t, h.start(t).URL)
			return nil
		}, "an answer the hook cannot read"},
		{"a slow server", func(t *testing.T) []string {
			h := &hookServer{answer: coveredAnswer, delay: 500 * time.Millisecond}
			hookEnv(t, h.start(t).URL)
			return []string{"--timeout", "50ms"}
		}, "did not answer"},
	}
	for _, c := range cases {
		for _, onError := range []string{"deny", "allow"} {
			t.Run(c.name+"/"+onError, func(t *testing.T) {
				extra := c.setup(t)
				args := append([]string{"--repo", "acme/app", "--root", root, "--on-error", onError}, extra...)
				stderr := captureStderr(t, func() {
					o, raw := runHook(t, args, call)
					if strings.Contains(raw, secret) {
						t.Errorf("the output carries the token: %s", raw)
					}
					text := o.HookSpecificOutput.PermissionDecisionReason + o.HookSpecificOutput.AdditionalContext
					if o.HookSpecificOutput.PermissionDecision != onError || !strings.Contains(text, c.want) {
						t.Errorf("decision %q, text %q; want %s with %q", o.HookSpecificOutput.PermissionDecision, text, onError, c.want)
					}
					if onError == "deny" && !strings.Contains(text, "change --on-error to allow") {
						t.Errorf("the deny does not say how to switch: %q", text)
					}
				})
				if strings.Contains(stderr, secret) {
					t.Errorf("stderr carries the token: %s", stderr)
				}
			})
		}
	}
}

func TestPreEditHook_BadInputDenies(t *testing.T) {
	h := &hookServer{answer: coveredAnswer}
	hookEnv(t, h.start(t).URL)
	root := t.TempDir()
	good := editCall("s", root, filepath.Join(root, "a.go"))
	cases := []struct {
		name  string
		args  []string
		input string
		want  string
	}{
		{"no --repo", []string{"--root", root}, good, "needs --repo"},
		{"a bad --on-error", []string{"--repo", "a/b", "--on-error", "maybe"}, good, "--on-error must be deny or allow"},
		{"an unknown flag", []string{"--repo", "a/b", "--nope"}, good, "flag provided but not defined"},
		{"stdin that is not JSON", []string{"--repo", "a/b", "--root", root}, "{", "not Claude Code's PreToolUse JSON"},
		{"stdin over the limit", []string{"--repo", "a/b", "--root", root}, strings.Repeat(" ", hookInputLimit+1), "could not read the tool call"},
		{"a call with no file", []string{"--repo", "a/b", "--root", root}, `{"tool_name":"Edit","tool_input":{}}`, "names no file"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o, _ := runHook(t, c.args, c.input)
			if o.HookSpecificOutput.PermissionDecision != "deny" || !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, c.want) {
				t.Errorf("%+v, want deny with %q", o, c.want)
			}
		})
	}
	if h.calls != 0 {
		t.Errorf("bad input reached the server %d times", h.calls)
	}
}

func TestPreEditHook_PanicIsADeny(t *testing.T) {
	hookEnv(t, "")
	var out bytes.Buffer
	var nilReader io.Reader
	runPreEditHook([]string{"--repo", "a/b"}, nilReader, &out)
	o := decodeHookOutput(t, out.String())
	if o.HookSpecificOutput.PermissionDecision != "deny" || !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, "the hook failed") {
		t.Errorf("%+v", o)
	}
}

func TestPreEditHook_NotebookAndOutsideRoot(t *testing.T) {
	h := &hookServer{answer: coveredAnswer}
	hookEnv(t, h.start(t).URL)
	root := t.TempDir()
	nb, _ := json.Marshal(map[string]any{"session_id": "s", "tool_name": "NotebookEdit", "tool_input": map[string]any{"notebook_path": filepath.Join(root, "nb", "a.ipynb")}})
	if o, _ := runHook(t, []string{"--repo", "acme/app", "--root", root}, string(nb)); o.HookSpecificOutput.PermissionDecision != "deny" ||
		fmt.Sprint(h.paths) != "[nb/a.ipynb]" {
		t.Errorf("notebook: %+v, paths %v", o, h.paths)
	}
	calls := h.calls
	o, _ := runHook(t, []string{"--repo", "acme/app", "--root", root}, editCall("s", root, filepath.Join(t.TempDir(), "elsewhere.go")))
	if o.HookSpecificOutput.PermissionDecision != "allow" || !strings.Contains(o.HookSpecificOutput.AdditionalContext, "outside the repository root") || h.calls != calls {
		t.Errorf("outside the root: %+v (calls %d -> %d)", o, calls, h.calls)
	}
	// Without --root: CLAUDE_PROJECT_DIR, then the session's cwd.
	t.Setenv("CLAUDE_PROJECT_DIR", root)
	if o, _ := runHook(t, []string{"--repo", "acme/app"}, editCall("p", "", filepath.Join(root, "x.go"))); o.HookSpecificOutput.PermissionDecision != "deny" ||
		fmt.Sprint(h.paths) != "[x.go]" {
		t.Errorf("CLAUDE_PROJECT_DIR: %+v, paths %v", o, h.paths)
	}
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	if o, _ := runHook(t, []string{"--repo", "acme/app"}, editCall("c", root, "rel/y.go")); o.HookSpecificOutput.PermissionDecision != "deny" ||
		fmt.Sprint(h.paths) != "[rel/y.go]" {
		t.Errorf("cwd and a relative path: %+v, paths %v", o, h.paths)
	}
}

func TestPreEditHook_TruncatedAnswerSaysSo(t *testing.T) {
	h := &hookServer{answer: strings.Replace(coveredAnswer, `"uncovered":[]`, `"uncovered":[],"truncated":true`, 1)}
	hookEnv(t, h.start(t).URL)
	root := t.TempDir()
	o, _ := runHook(t, []string{"--repo", "acme/app", "--root", root}, editCall("s", root, filepath.Join(root, "a.go")))
	if !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, "The list is not complete: run smeldr-cli decisions-for-paths --repo acme/app a.go") {
		t.Errorf("%+v", o)
	}
}

func TestPreEditHook_StateFile(t *testing.T) {
	h := &hookServer{answer: coveredAnswer}
	hookEnv(t, h.start(t).URL)
	root := t.TempDir()
	args := []string{"--repo", "acme/app", "--root", root}
	call := editCall("../../etc/x", root, filepath.Join(root, "a.go"))
	runHook(t, args, call)
	dir := os.Getenv("SMELDR_HOOK_STATE_DIR")
	file := filepath.Join(dir, "etcx.json")
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("the session's file is not %s: %v", file, err)
	}
	// A corrupt file is an empty set: denied again, never allowed by accident.
	if err := os.WriteFile(file, []byte("{half"), 0o600); err != nil {
		t.Fatal(err)
	}
	if o, _ := runHook(t, args, call); o.HookSpecificOutput.PermissionDecision != "deny" {
		t.Errorf("corrupt state: %+v", o)
	}
	if o, _ := runHook(t, args, call); o.HookSpecificOutput.PermissionDecision != "allow" {
		t.Errorf("after the rewrite: %+v", o)
	}
	// A session with no ID still gets one file.
	runHook(t, args, editCall("", root, filepath.Join(root, "a.go")))
	if _, err := os.Stat(filepath.Join(dir, "no-session.json")); err != nil {
		t.Errorf("no-session file: %v", err)
	}
	// Files older than a week are removed when the hook writes.
	old := filepath.Join(dir, "old.json")
	if err := os.WriteFile(old, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	runHook(t, args, editCall("fresh", root, filepath.Join(root, "a.go")))
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("a week-old file was kept: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".shown-") {
			t.Errorf("a temp file was left: %s", e.Name())
		}
	}
}

func TestRepoRelative(t *testing.T) {
	cases := []struct {
		root, file, cwd string
		want            string
		inside          bool
	}{
		{"/work/app", "/work/app/internal/a.go", "", "internal/a.go", true},
		{"/work/app/", "/work/app/a.go", "", "a.go", true},
		{"/work/app", "/work/apple/a.go", "", "", false},
		{"/work/app", "/work/app", "", "", false},
		{"/work/app", "/work/app/../other/a.go", "", "", false},
		{"/work/app", "rel/a.go", "/work/app/sub", "sub/rel/a.go", true},
		{"/work/app", "rel/a.go", "", "rel/a.go", true},
		{`C:\Work\App`, `c:\work\app\internal\billing\a.go`, "", "internal/billing/a.go", true},
		{"/c/Work/App", `C:\Work\App\a.go`, "", "a.go", true},
		{`C:\Work\App`, `D:\Work\App\a.go`, "", "", false},
		{`C:\`, `C:\a.go`, "", "a.go", true},
		{"/", "/a.go", "", "a.go", true},
		{"", "/a.go", "", "", false},
	}
	for _, c := range cases {
		got, inside := repoRelative(c.root, c.file, c.cwd)
		if got != c.want || inside != c.inside {
			t.Errorf("repoRelative(%q, %q, %q) = %q, %v; want %q, %v", c.root, c.file, c.cwd, got, inside, c.want, c.inside)
		}
	}
}

func TestHook_Help(t *testing.T) {
	for _, run := range []func(){
		func() { runHookCommand(nil) },
		func() { runHookCommand([]string{"--help"}) },
		func() { runPreEditHook([]string{"--help"}, strings.NewReader(""), io.Discard) },
	} {
		out := captureStdout(t, run)
		for _, want := range []string{"Claude Code's PreToolUse JSON", "not a general", "to this token", "--on-error deny|allow", "does not check the", "through the\nshell"} {
			if !strings.Contains(out, want) {
				t.Errorf("help is missing %q:\n%s", want, out)
			}
		}
	}
}
