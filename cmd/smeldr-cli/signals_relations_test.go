package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// core T1308: relation assert, decision place, signal get and list, the help
// lists every command, decision get takes an address, --from is read as LF.

func TestParseItemRef(t *testing.T) {
	for _, c := range []struct{ in, typ, id, err string }{
		{"D4", "Decision", "D4", ""},
		{"ARE12", "Area", "ARE12", ""},
		{"Decision:money-in-cents", "Decision", "money-in-cents", ""},
		{"note:N1", "note", "N1", ""},
		{"XYZ1", "", "", "Type:ident"},
		{"money", "", "", "Type:ident"},
		{":x", "", "", "Type:ident"},
	} {
		typ, id, err := parseItemRef(c.in)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%s: err %v, want %q", c.in, err, c.err)
			}
			continue
		}
		if err != nil || typ != c.typ || id != c.id {
			t.Errorf("%s = %s %s %v", c.in, typ, id, err)
		}
	}
}

func TestRelationAssert(t *testing.T) {
	tool, args, out := mockMCP(t, map[string]any{"id": "e1", "rev": 1}, func() {
		runRelationCommand([]string{"assert", "D4", "grounded_in", "Decision:D2", "--rev", "3"})
	})
	if tool != "assert_relation" {
		t.Fatalf("tool %q", tool)
	}
	want := map[string]any{"source_type": "Decision", "source_id": "D4", "relation_kind": "grounded_in", "target_type": "Decision", "target_id": "D2", "rev": float64(3)}
	for k, v := range want {
		if args[k] != v {
			t.Errorf("%s = %v, want %v", k, args[k], v)
		}
	}
	if !strings.Contains(out, "Decision:D4 grounded_in Decision:D2 asserted (relation e1, rev 1)") {
		t.Errorf("out = %q", out)
	}
	_, _, js := mockMCP(t, map[string]any{"id": "e1"}, func() {
		runRelationCommand([]string{"assert", "D4", "grounded_in", "D2", "--json"})
	})
	if !strings.Contains(js, `"id": "e1"`) {
		t.Errorf("--json out = %q", js)
	}
	for _, bad := range [][]string{{"assert", "D4", "grounded_in"}, {"assert", "money", "k", "D2"}, {"assert", "D4", "k", "D2", "--class", "observed"}} {
		if code, _ := runExpectingExit(t, func() { runRelationCommand(bad) }); code != 1 {
			t.Errorf("%v: exit %d", bad, code)
		}
	}
}

func TestDecisionPlace(t *testing.T) {
	for _, c := range []struct{ flag, typ, kind string }{
		{"--area", "Area", "belongs_to_area"}, {"--domain", "Domain", "belongs_to_domain"}, {"--organization", "Organization", "belongs_to_organization"},
	} {
		tool, args, out := mockMCP(t, map[string]any{"id": "e2"}, func() {
			runDecisionCommand([]string{"place", "D4", c.flag, "X1"})
		})
		if tool != "assert_relation" || args["relation_kind"] != c.kind || args["target_type"] != c.typ || args["target_id"] != "X1" || args["source_type"] != "Decision" || args["source_id"] != "D4" {
			t.Errorf("%s: %s %v", c.flag, tool, args)
		}
		if !strings.Contains(out, "asserted") {
			t.Errorf("%s: out %q", c.flag, out)
		}
	}
	for _, bad := range [][]string{{"place", "D4"}, {"place", "D4", "--area", "A", "--domain", "B"}, {"place", "--area", "A"}} {
		code, stderr := runExpectingExit(t, func() { runDecisionCommand(bad) })
		if code != 1 {
			t.Errorf("%v: exit %d (%s)", bad, code, stderr)
		}
	}
}

func TestSignalGet(t *testing.T) {
	tool, args, out := mockMCP(t, map[string]any{"address": "S7", "signal_type": "question", "Status": "answered", "receiver": "core",
		"subject_type": "Task", "subject_id": "t-1", "message": "Which?", "answer": "This one", "answer_ref": "Task:T4"}, func() {
		runSignalCommand([]string{"get", "S7"})
	})
	if tool != "get_signal" || args["slug"] != "S7" {
		t.Errorf("%s %v", tool, args)
	}
	for _, want := range []string{"Signal:    S7", "State:     answered", "Subject:   Task t-1", "Answer:    This one", "Ref:       Task:T4"} {
		if !strings.Contains(out, want) {
			t.Errorf("out lacks %q:\n%s", want, out)
		}
	}
}

func TestSignalList(t *testing.T) {
	page := map[string]any{"signals": []map[string]any{{"address": "S9", "status": "open", "signal_type": "ratification-required", "receiver": "ratify",
		"subject_type": "Decision", "subject_id": "d-1", "message": strings.Repeat("x", 80)}}, "count": 1, "total": 1}
	tool, args, out := mockMCP(t, page, func() { runSignalCommand([]string{"list"}) })
	if tool != "list_signals" || args["addressed_to_me"] != true || args["receiver"] != nil {
		t.Errorf("bare list: %s %v", tool, args)
	}
	if !strings.Contains(out, "S9") || !strings.Contains(out, "Decision d-1") || !strings.Contains(out, "1 of 1 Signals") || strings.Contains(out, strings.Repeat("x", 61)) {
		t.Errorf("out = %q", out)
	}
	_, args, _ = mockMCP(t, page, func() {
		runSignalCommand([]string{"list", "--receiver", "core", "--state", "open", "--limit", "5"})
	})
	if args["addressed_to_me"] != nil || args["receiver"] != "core" || args["state"] != "open" || args["limit"] != float64(5) {
		t.Errorf("with a receiver: %v", args)
	}
	// A row with no address (an older mcp): the slug names it, and no
	// column reads <nil>.
	old := map[string]any{"signals": []map[string]any{{"id": "01a1", "slug": "core-question-1", "status": "open", "signal_type": "question"}}, "count": 1, "total": 1}
	if _, _, out := mockMCP(t, old, func() { runSignalCommand([]string{"list", "--receiver", "core"}) }); !strings.Contains(out, "core-question-1") || strings.Contains(out, "<nil>") {
		t.Errorf("no address: %q", out)
	}
	if _, _, out := mockMCP(t, map[string]any{"signals": []any{}, "total": 0}, func() { runSignalCommand([]string{"list", "--sender", "core"}) }); !strings.Contains(out, "No Signals.") {
		t.Errorf("empty: %q", out)
	}
	for _, bad := range [][]string{{"list", "x"}, {"list", "--limit", "-1"}, {"list", "--bogus", "1"}} {
		if code, _ := runExpectingExit(t, func() { runSignalCommand(bad) }); code != 1 {
			t.Errorf("%v: exit %d", bad, code)
		}
	}
}

// An mcp without addressed_to_me: the bare list names --receiver.
func TestSignalList_OlderServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1,
			"error": map[string]any{"code": -32602, "message": "invalid params: unknown parameter: addressed_to_me"}})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SMELDR_URL", srv.URL)
	t.Setenv("SMELDR_TOKEN", "test-token")
	t.Setenv("SMELDR_MCP_URL", srv.URL)
	code, stderr := runExpectingExit(t, func() { runSignalCommand([]string{"list"}) })
	if code != 1 || !strings.Contains(stderr, "--receiver") {
		t.Errorf("exit %d: %s", code, stderr)
	}
}

// decision get: a slug the route finds is read there, unchanged; an address
// the route does not find is read with get_decision.
func TestDecisionGet(t *testing.T) {
	var mcpTool string
	var mcpArgs map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/decisions/money-in-cents":
			_, _ = io.WriteString(w, `{"ID":"d-1","Slug":"money-in-cents","Status":"ratified","title":"Money in cents"}`)
		case strings.HasPrefix(r.URL.Path, "/decisions/"):
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		case r.URL.Path == "/mcp":
			var req struct {
				Params struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments"`
				} `json:"params"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			mcpTool, mcpArgs = req.Params.Name, req.Params.Arguments
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{
				"content": []map[string]any{{"type": "text", "text": `{"ID":"d-1","address":"D4","standing":"holds"}`}}}})
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SMELDR_URL", srv.URL)
	t.Setenv("SMELDR_TOKEN", "test-token")
	t.Setenv("SMELDR_MCP_URL", srv.URL+"/mcp")

	out := captureStdout(t, func() { runDecisionCommand([]string{"get", "money-in-cents"}) })
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil || got["Slug"] != "money-in-cents" || got["title"] != "Money in cents" || mcpTool != "" {
		t.Errorf("slug: %q (%v), MCP called %q", out, err, mcpTool)
	}
	out = captureStdout(t, func() { runDecisionCommand([]string{"get", "D4"}) })
	if mcpTool != "get_decision" || mcpArgs["slug"] != "D4" || !strings.Contains(out, `"address": "D4"`) {
		t.Errorf("address: %s %v %q", mcpTool, mcpArgs, out)
	}
}

func TestDecisionFileHelp(t *testing.T) {
	out := captureStdout(t, func() { runDecisionCommand([]string{"create", "--help"}) })
	if !strings.Contains(out, "smeldr-cli decision create --from <file>") || !strings.Contains(out, "title: ") || strings.Contains(out, "decisions create") {
		t.Errorf("create --help = %q", out)
	}
	out = captureStdout(t, func() { runDecisionCommand([]string{"update", "-h"}) })
	if !strings.Contains(out, "smeldr-cli decision update <slug> --from <file>") {
		t.Errorf("update --help = %q", out)
	}
}

// The top-level help names every command the dispatcher knows, and every verb
// of decision, relation and signal.
func TestUsage_ListsEveryCommand(t *testing.T) {
	// The help is longer than a pipe's buffer, so it goes to a file.
	f, err := os.CreateTemp(t.TempDir(), "usage")
	if err != nil {
		t.Fatal(err)
	}
	printUsage(f)
	_ = f.Close()
	raw, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	usage := string(raw)
	caseWord := regexp.MustCompile(`case "([a-z][a-z-]*)"`)
	words := func(file, fn string) []string {
		src, err := os.ReadFile(filepath.Clean(file))
		if err != nil {
			t.Fatal(err)
		}
		s := strings.ReplaceAll(string(src), "\r\n", "\n")
		i := strings.Index(s, "func "+fn+"(")
		if i < 0 {
			t.Fatalf("%s not in %s", fn, file)
		}
		body := s[i : i+strings.Index(s[i:], "\n}\n")]
		var out []string
		for _, m := range caseWord.FindAllStringSubmatch(body, -1) {
			out = append(out, m[1])
		}
		return out
	}
	for _, w := range words("main.go", "main") {
		if w == "help" || w == "version" {
			continue
		}
		if !strings.Contains(usage, "smeldr-cli "+w+" ") {
			t.Errorf("printUsage does not list %q", w)
		}
	}
	verbs := map[string][]string{
		"decision": append(words("decision.go", "runDecisionCommand"), "list"),
		"relation": {"list", "assert", "withdraw"},
		"signal":   {"list", "get", "answer", "withdraw"},
	}
	for cmd, vs := range verbs {
		for _, v := range vs {
			re := regexp.MustCompile(`smeldr-cli ` + cmd + ` [^\n]*\b` + v + `\b`)
			if !re.MatchString(usage) {
				t.Errorf("printUsage does not list %s %s", cmd, v)
			}
		}
	}
}

// --from files are read with LF line endings.
func TestFrom_LineEndings(t *testing.T) {
	for name, content := range map[string]string{
		"all CRLF":    "---\r\ntitle: T\r\nscope: s\r\n---\r\nBody line\r\nsecond\r\n",
		"here-string": "---\ntitle: T\nscope: s\n---\nBody line\nsecond\r\n",
		"lone CR":     "---\rtitle: T\rscope: s\r---\rBody line\rsecond\r",
	} {
		path := filepath.Join(t.TempDir(), "d.md")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		fields, body, err := parseFrontmatterFile(path)
		if err != nil || fields["title"] != "T" || fields["scope"] != "s" || body != "Body line\nsecond\n" {
			t.Errorf("%s: %v %q %v", name, fields, body, err)
		}
	}
	path := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(path, []byte("[\r\n]\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := readFromFile(path); err != nil || string(data) != "[\n]\n" {
		t.Errorf("json: %q %v", data, err)
	}
	if _, err := readFromFile(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing file read")
	}
}
