package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const bom = "\xEF\xBB\xBF"

// setStdin feeds s to the package's stdin for the test.
func setStdin(t *testing.T, s string) {
	t.Helper()
	old := stdin
	stdin = strings.NewReader(s)
	t.Cleanup(func() { stdin = old })
}

// T1238: Windows PowerShell 5.1 at code page 65001 pipes one byte order mark
// in front, two when $OutputEncoding is UTF-8 too (measured: ef bb bf, then
// ef bb bf ef bb bf). They are dropped, and nothing after them is touched.
func TestWithoutBOM(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"tok123\r\n", "tok123\r\n"},
		{bom + "tok123\r\n", "tok123\r\n"},
		{bom + bom + "tok123\r\n", "tok123\r\n"},
		{bom, ""},
		{"", ""},
		{"a" + bom + "b", "a" + bom + "b"},
		{"\xEF\xBB", "\xEF\xBB"},
	} {
		got, err := io.ReadAll(withoutBOM(strings.NewReader(c.in)))
		if err != nil || string(got) != c.want {
			t.Errorf("withoutBOM(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
		if got := string(trimBOM([]byte(c.in))); got != c.want {
			t.Errorf("trimBOM(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// T1238: a secret from its variable is trimmed as the stdin one is, and a
// byte order mark in front of either is dropped; the secret is never shown.
func TestSecretInput_TrimAndBOM(t *testing.T) {
	src := socialClientSecret
	for _, c := range []struct {
		name string
		env  *string
		args []string
		in   string
	}{
		{"env with a trailing space and newline", ptr(testSecret + " \n"), nil, ""},
		{"env with a tab in front", ptr("\t" + testSecret), nil, ""},
		{"env with a byte order mark", ptr(bom + testSecret), nil, ""},
		{"stdin with one byte order mark", nil, []string{"--client-secret-stdin"}, bom + testSecret + "\r\n"},
		{"stdin with two byte order marks", nil, []string{"--client-secret-stdin"}, bom + bom + testSecret + "\r\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.env != nil {
				t.Setenv(src.env, *c.env)
			} else {
				unsetEnv(t, src.env)
			}
			setStdin(t, c.in)
			fs := newCommandFlags("t")
			value, fromStdin := src.secretFlags(fs, "the secret")
			if err := fs.Parse(c.args); err != nil {
				t.Fatal(err)
			}
			var got string
			var err error
			_, stderr := runExpectingExit(t, func() { got, err = src.read(fs, *value, *fromStdin) })
			if err != nil || got != testSecret {
				t.Errorf("got %q, %v; want the secret", got, err)
			}
			if strings.Contains(stderr, testSecret) {
				t.Errorf("stderr shows the secret: %q", stderr)
			}
		})
	}
	// A variable that is only white space and a byte order mark is still empty.
	t.Setenv(src.env, bom+" \t\n")
	fs := newCommandFlags("t")
	value, fromStdin := src.secretFlags(fs, "the secret")
	if _, err := src.read(fs, *value, *fromStdin); err == nil || !strings.Contains(err.Error(), "$SMELDR_SOCIAL_CLIENT_SECRET is empty") {
		t.Errorf("blank env: %v", err)
	}
}

// T1238: --stdin paths from Windows PowerShell 5.1 carry no byte order mark
// to the server.
func TestDecisionsForPaths_StdinWithBOM(t *testing.T) {
	setStdin(t, bom+bom+"internal/billing/a.go\r\ndocs/b.md\r\n")
	result := map[string]any{"areas": []any{}, "decisions": []any{}, "uncovered": []any{"internal/billing/a.go", "docs/b.md"}}
	_, args, _ := mockMCP(t, result, func() { runDecisionsForPathsCommand([]string{"--repo", "acme/app", "--stdin"}) })
	if fmt.Sprint(args["paths"]) != "[internal/billing/a.go docs/b.md]" {
		t.Errorf("paths sent = %q", args["paths"])
	}
}

// T1238: a file written by Windows PowerShell 5.1's Set-Content -Encoding
// utf8 starts with a byte order mark; the cli reads it as if it did not.
func TestFilesAndStdinWithBOM(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	front := "---\ntitle: Hello\n---\nBody\n"
	fields, body, err := parseFrontmatterFile(write("post.md", bom+front))
	if err != nil || fields["title"] != "Hello" || !strings.Contains(body, "Body") {
		t.Errorf("frontmatter file: %v %q %v", fields, body, err)
	}
	setStdin(t, bom+front)
	if fields, _, err := parseFrontmatterFile("-"); err != nil || fields["title"] != "Hello" {
		t.Errorf("frontmatter stdin: %v %v", fields, err)
	}
	if data, err := readFromFile(write("basis.md", bom+"basis")); err != nil || string(data) != "basis" {
		t.Errorf("readFromFile file: %q %v", data, err)
	}
	setStdin(t, bom+"basis")
	if data, err := readFromFile("-"); err != nil || string(data) != "basis" {
		t.Errorf("readFromFile stdin: %q %v", data, err)
	}
	if f, err := readFilter("@" + write("filter.json", bom+`{"types":["Task"]}`)); err != nil || f["types"] == nil {
		t.Errorf("readFilter file: %v %v", f, err)
	}
	if data, err := readFileNoBOM(write("plain.json", "[]")); err != nil || string(data) != "[]" {
		t.Errorf("readFileNoBOM plain: %q %v", data, err)
	}
	if _, err := readFileNoBOM(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing file must be an error")
	}
}

// T1238: a "y" typed into a UTF-8 console confirms; a byte order mark does
// not turn it into a no.
func TestConfirmRev_BOM(t *testing.T) {
	setStdin(t, bom+"y\r\n")
	code, _ := runExpectingExit(t, func() { confirmRev("transition", "Decision D1", 3) })
	if code != -1 {
		t.Errorf("exit %d, want no exit", code)
	}
}

// T1238: the hook reads a tool call with a byte order mark in front.
func TestPreEditHook_BOMOnStdin(t *testing.T) {
	h := &hookServer{answer: coveredAnswer}
	hookEnv(t, h.start(t).URL)
	root := t.TempDir()
	o, _ := runHook(t, []string{"--repo", "acme/app", "--root", root}, bom+editCall("s", root, filepath.Join(root, "a.go")))
	if o.HookSpecificOutput.PermissionDecision != "deny" || !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, "Decision D1") {
		t.Errorf("%+v", o)
	}
}

// T1270: a covered file is denied, never "outside the root", when the root
// is missing or relative; v0.39.0 allowed these edits.
func TestPreEditHook_RootIsAlwaysAbsolute(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "internal", "billing", "a.go")
	old := hookGetwd
	t.Cleanup(func() { hookGetwd = old })
	hookGetwd = func() (string, error) { return root, nil }
	for name, c := range map[string]struct {
		args []string
		env  string
		cwd  string
	}{
		"no root from anywhere":         {nil, "", ""},
		"--root .":                      {[]string{"--root", "."}, "", ""},
		"a relative CLAUDE_PROJECT_DIR": {nil, ".", ""},
	} {
		t.Run(name, func(t *testing.T) {
			h := &hookServer{answer: coveredAnswer}
			hookEnv(t, h.start(t).URL)
			t.Setenv("CLAUDE_PROJECT_DIR", c.env)
			args := append([]string{"--repo", "acme/app"}, c.args...)
			o, _ := runHook(t, args, editCall("s-"+name, c.cwd, file))
			if o.HookSpecificOutput.PermissionDecision != "deny" || fmt.Sprint(h.paths) != "[internal/billing/a.go]" {
				t.Errorf("%+v, paths %v", o, h.paths)
			}
		})
	}
	// A file outside an absolute root is still an allow.
	h := &hookServer{answer: coveredAnswer}
	hookEnv(t, h.start(t).URL)
	o, _ := runHook(t, []string{"--repo", "acme/app", "--root", root}, editCall("s", "", filepath.Join(t.TempDir(), "x.go")))
	if o.HookSpecificOutput.PermissionDecision != "allow" || !strings.Contains(o.HookSpecificOutput.AdditionalContext, "outside the repository root") {
		t.Errorf("outside: %+v", o)
	}
	// No working directory to fall back on: the error path, a deny.
	hookGetwd = func() (string, error) { return "", fmt.Errorf("gone") }
	for _, args := range [][]string{{"--repo", "acme/app"}, {"--repo", "acme/app", "--root", "."}} {
		o, _ := runHook(t, args, editCall("s", "", file))
		if o.HookSpecificOutput.PermissionDecision != "deny" || !strings.Contains(o.HookSpecificOutput.PermissionDecisionReason, "could not") {
			t.Errorf("%v: %+v", args, o)
		}
	}
}

// T1270: a token that reads an Area but not its Domain or the Organization
// is told what it saw, not that no Decision is in force.
func TestPreEditHook_SaysWhatTheTokenSaw(t *testing.T) {
	h := &hookServer{answer: `{"areas":[{"address":"ARE2","name":"Ledger","approval_mode":"comment-only"}],"decisions":[],"uncovered":[]}`}
	hookEnv(t, h.start(t).URL)
	root := t.TempDir()
	o, _ := runHook(t, []string{"--repo", "acme/app", "--root", root}, editCall("s", root, filepath.Join(root, "a.go")))
	r := o.HookSpecificOutput.PermissionDecisionReason
	if !strings.Contains(r, "Smeldr named no Decision in force for this file to this token.") ||
		!strings.Contains(r, "A place this token may not read is left out, with its Decisions.") ||
		strings.Contains(r, "No Decision is in force") {
		t.Errorf("reason:\n%s", r)
	}
}

// T1238 point 3: relation list shows the rev withdraw needs, and the help of
// every command that takes --rev names it.
func TestRevShownAndNamed(t *testing.T) {
	var buf bytes.Buffer
	printRelationTable(&buf, relationListPage{Edges: []map[string]any{
		{"id": "e1", "rev": float64(3), "relation_kind": "contains", "source_type": "Goal", "source_id": "g", "target_type": "Task", "target_id": "t", "edge_class": "asserted"},
		{"id": "e2", "relation_kind": "contains", "source_type": "Goal", "source_id": "g", "target_type": "Task", "target_id": "u", "edge_class": "asserted"},
	}, Total: 2, AsOf: "2026-11-15T00:00:00Z"})
	out := buf.String()
	if !strings.Contains(out, "ID  REV  KIND") || !strings.Contains(out, "e1  3    contains") || !strings.Contains(out, "e2  -    contains") {
		t.Errorf("table:\n%s", out)
	}
	for name, help := range map[string]func(){
		"relation": printRelationHelp,
		"check":    printCheckHelp,
		"premise":  printPremiseHelp,
	} {
		if text := captureStdout(t, help); !strings.Contains(text, "--rev") {
			t.Errorf("%s help does not name --rev:\n%s", name, text)
		}
	}
	if text := captureStdout(t, printRelationHelp); !strings.Contains(text, "withdraw <id> --rev <n> --reason") || !strings.Contains(text, "REV column") {
		t.Errorf("relation help:\n%s", text)
	}
}
