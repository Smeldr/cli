package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// core T1292: items of a runtime-defined type over /_content/{type}, and the
// prefix form left as it was.

// fakeContentServer answers a runtime type "recipe" (items r1/pasta,
// r2/soup) over /_content, a compiled module at /content, and records every
// request as "METHOD path?query body".
type fakeContentServer struct {
	mu    sync.Mutex
	calls []string
	total int // when set, the list answers this total
}

func (f *fakeContentServer) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	call := r.Method + " " + r.URL.Path
	if r.URL.RawQuery != "" {
		call += "?" + r.URL.RawQuery
	}
	if len(body) > 0 {
		call += " " + string(body)
	}
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
	items := []map[string]any{
		{"ID": "r1", "Slug": "pasta", "Status": "draft", "title": "Pasta"},
		{"ID": "r2", "Slug": "soup", "Status": "published", "title": "Soup"},
	}
	switch {
	case r.URL.Path == "/_content/recipe" && r.Method == http.MethodGet:
		total := len(items)
		if f.total > 0 {
			total = f.total
		}
		if s := r.URL.Query().Get("status"); s != "" {
			kept := items[:0]
			for _, it := range items {
				if it["Status"] == s {
					kept = append(kept, it)
				}
			}
			items, total = kept, len(kept)
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items, "total": total}) //nolint:errcheck
	case r.URL.Path == "/_content/recipe" && r.Method == http.MethodPost:
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"ID":"r3","Slug":"new"}`)
	case strings.HasPrefix(r.URL.Path, "/_content/recipe/r1") || strings.HasPrefix(r.URL.Path, "/_content/recipe/r2"):
		fmt.Fprint(w, `{"ID":"r1","Slug":"pasta","Status":"draft"}`)
	case strings.HasPrefix(r.URL.Path, "/_content/secret"):
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"type":"https://smeldr.dev/problems/role.insufficient","detail":"your role may not read secret"}`)
	case strings.HasPrefix(r.URL.Path, "/_content/"):
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"detail":"not found"}`)
	case r.URL.Path == "/content":
		fmt.Fprint(w, `[{"Slug":"about","Status":"published"}]`)
	case r.URL.Path == "/content/about":
		fmt.Fprint(w, `{"Slug":"about","Status":"published"}`)
	case r.URL.Path == "/recipes" && r.Method == http.MethodGet:
		fmt.Fprint(w, `{"items":[{"Slug":"soup","Status":"published"}],"total":1}`)
	case r.URL.Path == "/gone" || strings.HasPrefix(r.URL.Path, "/gone/"):
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"detail":"route not found"}`)
	case r.URL.Path == "/recipes/soup" && r.Method == http.MethodPut, r.URL.Path == "/recipes" && r.Method == http.MethodPost:
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprint(w, "Method Not Allowed")
	case r.URL.Path == "/recipes/soup":
		fmt.Fprint(w, `{"Slug":"soup","Status":"published"}`)
	case r.URL.Path == "/broken":
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "boom")
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newFakeContent(t *testing.T) *fakeContentServer {
	t.Helper()
	f := &fakeContentServer{}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	t.Setenv("SMELDR_URL", srv.URL)
	t.Setenv("SMELDR_TOKEN", "test-token")
	return f
}

func (f *fakeContentServer) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// runMain runs the cli's main with args and returns stdout, stderr and the exit
// code (-1 when it returned).
func runMain(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	old := os.Args
	os.Args = append([]string{"smeldr-cli"}, args...)
	defer func() { os.Args = old }()
	var out string
	code, errOut := runExpectingExit(t, func() {
		out = captureStdoutPanicSafe(t, main)
	})
	return out, errOut, code
}

// captureStdoutPanicSafe is captureStdout that restores stdout when fn exits
// through the test's osExit panic.
func captureStdoutPanicSafe(t *testing.T, fn func()) (out string) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	defer func() {
		_ = w.Close()
		os.Stdout = old
		out = <-done
	}()
	fn()
	return ""
}

func frontMatterFile(t *testing.T, text string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "item.md")
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRuntimeContent_List(t *testing.T) {
	f := newFakeContent(t)
	out, _, code := runMain(t, "content", "recipe", "list")
	if code != -1 || !strings.Contains(out, "pasta") || !strings.Contains(out, "soup") || !strings.Contains(out, "SLUG") {
		t.Errorf("list: %d %s", code, out)
	}
	out, _, _ = runMain(t, "content", "recipe", "list", "--status", "draft", "--json")
	if !strings.Contains(out, `"Slug": "pasta"`) || strings.Contains(out, "soup") {
		t.Errorf("list --status draft --json: %s", out)
	}
	calls := f.got()
	if calls[0] != "GET /_content/recipe" || calls[1] != "GET /_content/recipe?status=draft" {
		t.Errorf("requests %v", calls)
	}
}

// A total above what came back is said, and a slug lookup on a cut list
// never says the item does not exist.
func TestRuntimeContent_CutList(t *testing.T) {
	f := newFakeContent(t)
	f.total = 9
	_, errOut, _ := runMain(t, "content", "recipe", "list")
	if !strings.Contains(errOut, "the server answered 2 of 9 items") {
		t.Errorf("cut list: %s", errOut)
	}
	_, errOut, code := runMain(t, "content", "recipe", "get", "missing")
	if code != 1 || !strings.Contains(errOut, `among the 2 of 9 items the server answered; name it by its ID`) {
		t.Errorf("miss on a cut list: %d %s", code, errOut)
	}
}

func TestRuntimeContent_GetBySlugOrID(t *testing.T) {
	f := newFakeContent(t)
	if out, _, code := runMain(t, "content", "recipe", "get", "r1"); code != -1 || !strings.Contains(out, `"pasta"`) {
		t.Errorf("get by ID: %d %s", code, out)
	}
	if out, _, code := runMain(t, "content", "recipe", "get", "soup"); code != -1 || !strings.Contains(out, `"ID"`) {
		t.Errorf("get by slug: %d %s", code, out)
	}
	want := []string{"GET /_content/recipe/r1", "GET /_content/recipe/r1", "GET /_content/recipe/soup", "GET /_content/recipe", "GET /_content/recipe/r2"}
	if got := f.got(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("requests %v, want %v", got, want)
	}
	if _, errOut, code := runMain(t, "content", "recipe", "get", "nope"); code != 1 || !strings.Contains(errOut, `no recipe item with slug or ID "nope"`) {
		t.Errorf("unknown: %d %s", code, errOut)
	}
}

func TestRuntimeContent_Writes(t *testing.T) {
	f := newFakeContent(t)
	file := frontMatterFile(t, "---\ntitle: Soup\n---\nHot water.\n")
	if _, errOut, code := runMain(t, "content", "recipe", "create", "--from", file); code != -1 {
		t.Fatalf("create: %d %s", code, errOut)
	}
	if _, errOut, code := runMain(t, "content", "recipe", "update", "pasta", "--from", frontMatterFile(t, "---\ntitle: Pasta 2\n---\n")); code != -1 {
		t.Fatalf("update: %d %s", code, errOut)
	}
	for _, verb := range []string{"publish", "unpublish", "archive"} {
		if _, errOut, code := runMain(t, "content", "recipe", verb, "r1"); code != -1 {
			t.Fatalf("%s: %d %s", verb, code, errOut)
		}
	}
	got := strings.Join(f.got(), "\n")
	for _, want := range []string{
		`POST /_content/recipe {"body":"Hot water.\n","title":"Soup"}`,
		`PATCH /_content/recipe/r1 {"title":"Pasta 2"}`,
		`POST /_content/recipe/r1/status {"status":"published"}`,
		`POST /_content/recipe/r1/status {"status":"draft"}`,
		`POST /_content/recipe/r1/status {"status":"archived"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing request %s in\n%s", want, got)
		}
	}
}

func TestRuntimeContent_Refusals(t *testing.T) {
	newFakeContent(t)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"content", "recipe", "delete", "r1"}, "cannot be deleted: the server has no delete route; archive the item instead"},
		{[]string{"content", "recipe", "drop"}, `unknown verb "drop" for content recipe`},
		{[]string{"content", "secret", "list"}, `server returned 403: {"type":"https://smeldr.dev/problems/role.insufficient","detail":"your role may not read secret"}`},
		{[]string{"content", "recipe", "create"}, "--from <file> is required for create"},
		{[]string{"content", "--type"}, "content --type requires a type name"},
		{[]string{"content", "recipe"}, "smeldr-cli content <type> <verb>"},
	} {
		if _, errOut, code := runMain(t, c.args...); code != 1 || !strings.Contains(errOut, c.want) {
			t.Errorf("%v: %d %q, want %q", c.args, code, errOut, c.want)
		}
	}
	if out, _, code := runMain(t, "content", "--help"); code != -1 || !strings.Contains(out, "content --type <name> <verb>") || !strings.Contains(out, "There is no delete") {
		t.Errorf("help: %d %s", code, out)
	}
}

// The prefix form is pinned: content <verb> reaches /content exactly as
// before, a type named like a verb needs --type, and /<prefix> always means
// a prefix.
func TestContentWord_PrefixFormUnchanged(t *testing.T) {
	f := newFakeContent(t)
	if out, _, code := runMain(t, "content", "list"); code != -1 || !strings.Contains(out, "about") {
		t.Errorf("content list: %d %s", code, out)
	}
	if out, _, code := runMain(t, "content", "get", "about"); code != -1 || !strings.Contains(out, "about") {
		t.Errorf("content get about: %d %s", code, out)
	}
	if _, _, code := runMain(t, "/content", "list"); code != -1 {
		t.Errorf("/content list: %d", code)
	}
	// content list list is the prefix form with a stray argument: refused,
	// with no request, as before.
	if _, _, code := runMain(t, "content", "list", "list"); code != 1 {
		t.Errorf("content list list: %d, want the refusal", code)
	}
	runMain(t, "content", "--type", "list", "list")
	want := []string{"GET /content", "GET /content/about", "GET /content", "GET /_content/list"}
	if got := f.got(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("requests %v, want %v", got, want)
	}
}

// A runtime type's public list answers an object; the prefix list reads it.
func TestRunList_ObjectAnswer(t *testing.T) {
	newFakeContent(t)
	out, errOut, code := runMain(t, "recipes", "list")
	if code != -1 || !strings.Contains(out, "soup") {
		t.Errorf("recipes list: %d %s %s", code, out, errOut)
	}
	if _, err := decodeListItems([]byte(`{"total":1}`)); err == nil {
		t.Error("an object without items decoded")
	}
}

// After a 404 or 405 the server's text comes first, then one hint line; any
// other status gets no hint.
func TestPrefixFailure_Hint(t *testing.T) {
	newFakeContent(t)
	file := frontMatterFile(t, "---\ntitle: X\n---\n")
	for _, c := range []struct {
		args []string
		text string
		verb string
	}{
		{[]string{"gone", "list"}, `server returned 404: {"detail":"route not found"}`, "list"},
		{[]string{"gone", "get", "x"}, `server returned 404: {"detail":"route not found"}`, "get"},
		{[]string{"gone", "update", "x", "--from", file}, `server returned 404: {"detail":"route not found"}`, "update"},
		{[]string{"gone", "publish", "x"}, `server returned 404: {"detail":"route not found"}`, "publish"},
		{[]string{"gone", "delete", "x"}, `server returned 404: {"detail":"route not found"}`, "delete"},
		{[]string{"recipes", "create", "--from", file}, "server returned 405: Method Not Allowed", "create"},
		{[]string{"recipes", "archive", "soup"}, "server returned 405: Method Not Allowed", "archive"},
	} {
		_, errOut, code := runMain(t, c.args...)
		hint := "If " + c.args[0] + " is a runtime-defined type or its public route, use: smeldr-cli content <type> " + c.verb
		ti, hi := strings.Index(errOut, c.text), strings.Index(errOut, hint)
		if code != 1 || ti < 0 || hi < 0 || hi < ti {
			t.Errorf("%v: %d %q", c.args, code, errOut)
		}
	}
	if _, errOut, _ := runMain(t, "broken", "list"); !strings.Contains(errOut, "server returned 500: boom") || strings.Contains(errOut, "If broken") {
		t.Errorf("500: %q", errOut)
	}
}

func TestSchemaToolsHint(t *testing.T) {
	err := fmt.Errorf("MCP error -32601: unknown tool: get_content_type_schema")
	if h := schemaToolsHint("get_content_type_schema", err); !strings.Contains(h, "ENABLE_DYNAMIC_CONTENT, ENABLE_SCHEMA_TOOLS or ENABLE_BLOCKS") {
		t.Errorf("hint %q", h)
	}
	if schemaToolsHint("define_content_type", err) != "" || schemaToolsHint("get_content_type_schema", fmt.Errorf("MCP error -32001: forbidden")) != "" {
		t.Error("a hint for another tool or another error")
	}
}
