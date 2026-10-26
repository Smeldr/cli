package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exited is what a test's osExit panics with.
type exited struct{ code int }

// runExpectingExit runs fn with osExit replaced, and returns the exit code
// (-1 when fn returned without exiting) and what fn wrote to stderr.
func runExpectingExit(t *testing.T, fn func()) (code int, stderr string) {
	t.Helper()
	oldExit, oldErr := osExit, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	osExit = func(c int) { panic(exited{c}) }
	code = -1
	func() {
		defer func() {
			if v := recover(); v != nil {
				e, ok := v.(exited)
				if !ok {
					panic(v)
				}
				code = e.code
			}
		}()
		fn()
	}()
	osExit, os.Stderr = oldExit, oldErr
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return code, string(out)
}

// TestCommandFlags_Refused (T959): every flag.FlagSet command refuses an
// unknown flag, a stray argument and a missing argument with exit 1, before
// any request (no server is set up), and -h exits 0.
func TestCommandFlags_Refused(t *testing.T) {
	t.Setenv("SMELDR_URL", "http://127.0.0.1:1")
	t.Setenv("SMELDR_TOKEN", "test-token")
	for _, tc := range []struct {
		name string
		run  func()
		code int
		want string
	}{
		{"create, stray", func() { runCreate("posts", []string{"x", "--from", "f.md"}) }, 1, `create: unexpected argument "x"`},
		{"update, two slugs", func() { runUpdate("posts", []string{"a", "b", "--from", "f.md"}) }, 1, `update: unexpected argument "b"`},
		{"update, no slug", func() { runUpdate("posts", []string{"--from", "f.md"}) }, 1, "update requires a slug argument"},
		{"update, unknown flag", func() { runUpdate("posts", []string{"a", "--form", "f.md"}) }, 1, "flag provided but not defined: -form"},
		{"publish, two slugs", func() { runLifecycle("posts", "publish", []string{"a", "b"}) }, 1, `publish: unexpected argument "b"`},
		{"delete, two slugs", func() { runDelete("posts", []string{"a", "b"}) }, 1, `delete: unexpected argument "b"`},
		{"delete, no slug", func() { runDelete("posts", nil) }, 1, "delete requires a slug argument"},
		{"get, two slugs", func() { runGet("posts", []string{"a", "b"}) }, 1, `get: unexpected argument "b"`},
		{"list, stray before a flag", func() { runList("posts", []string{"x", "--json"}) }, 1, `list: unexpected argument "x"`},
		{"audit list, stray", func() { runAuditCommand([]string{"list", "x", "--type", "post"}) }, 1, `audit list: unexpected argument "x"`},
		{"init, stray", func() { runInit([]string{"x", "--bootstrap-token", "t"}) }, 1, `init: unexpected argument "x"`},
		{"logs, stray", func() { runLogsCommand([]string{"x", "--json"}) }, 1, `logs: unexpected argument "x"`},
		{"status, stray", func() { runStatus([]string{"x"}) }, 1, `status: unexpected argument "x"`},
		{"token list, stray", func() { runTokenCommand([]string{"list", "x"}) }, 1, `token list: unexpected argument "x"`},
		{"node create, stray", func() { runNodeCommand([]string{"create", "x", "--type", "hero"}) }, 1, `block node create: unexpected argument "x"`},
		{"node update, two ids", func() { runNodeCommand([]string{"update", "n1", "n2", "--field", "A=1"}) }, 1, `block node update: unexpected argument "n2"`},
		{"node update, no id", func() { runNodeCommand([]string{"update", "--field", "A=1"}) }, 1, "block node update requires <id>"},
		{"node list, stray", func() { runNodeCommand([]string{"list", "x", "--json"}) }, 1, `block node list: unexpected argument "x"`},
		{"help exits 0", func() { runList("posts", []string{"-h"}) }, 0, "Usage: smeldr-cli posts list"},
		{"help after an argument exits 0", func() { runDelete("posts", []string{"a", "--help"}) }, 0, "Usage: smeldr-cli posts delete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, stderr := runExpectingExit(t, tc.run)
			if code != tc.code || !strings.Contains(stderr, tc.want) {
				t.Errorf("exit %d, stderr %q; want exit %d with %q", code, stderr, tc.code, tc.want)
			}
		})
	}
}

// TestCommandFlags_AnyOrder (T959): flags before, after or among the
// arguments reach the request.
func TestCommandFlags_AnyOrder(t *testing.T) {
	dir := t.TempDir()
	from := filepath.Join(dir, "post.md")
	if err := os.WriteFile(from, []byte("---\nTitle: New\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"post-one", "--from", from}, {"--from", from, "post-one"}} {
		var put string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut {
				b, _ := io.ReadAll(r.Body)
				put = r.URL.Path + " " + string(b)
			}
			fmt.Fprint(w, `{"Slug":"post-one","Title":"Old","Status":"draft"}`)
		}))
		t.Setenv("SMELDR_URL", srv.URL)
		t.Setenv("SMELDR_TOKEN", "test-token")
		captureStdout(t, func() { runUpdate("posts", args) })
		srv.Close()
		if !strings.HasPrefix(put, "/posts/post-one ") || !strings.Contains(put, `"Title":"New"`) {
			t.Errorf("update %v: PUT %q", args, put)
		}
	}
	out := mockHTTPLogs(t, 200, sampleContentListBody, func() { runList("posts", []string{"--status", "draft", "--json"}) })
	if !strings.Contains(out, "post-two") || strings.Contains(out, "post-one") {
		t.Errorf("list --status draft --json:\n%s", out)
	}
	_, args, _ := mockMCP(t, map[string]any{"ID": "n1"}, func() {
		runNodeCommand([]string{"update", "--field", "A=1", "n1", "--field", "B=2"})
	})
	fields, _ := args["fields"].(map[string]any)
	if args["id"] != "n1" || fields["A"] != "1" || fields["B"] != "2" {
		t.Errorf("node update, mixed: %v", args)
	}
}

// TestHandRolledReparse_Mixed (T959): the commands that re-parsed by hand
// failed on a flag between two arguments; they now read it.
func TestHandRolledReparse_Mixed(t *testing.T) {
	tool, params, err := parseGrantArgs([]string{"tok-1", "--reason", "owns docs", "steward"})
	if err != nil || tool != "grant_role" || params["role"] != "steward" || params["reason"] != "owns docs" {
		t.Errorf("grant, mixed: %q %v %v", tool, params, err)
	}
	name, role, ttl, class, reason, err := parseTokenCreateArgs([]string{"bot", "--class", "job", "editor", "30", "--reason", "ci"})
	if err != nil || name != "bot" || role != "editor" || ttl != 30 || class != "job" || reason != "ci" {
		t.Errorf("token create, mixed: %q %q %d %q %q %v", name, role, ttl, class, reason, err)
	}
	if _, _, _, _, _, err := parseTokenCreateArgs([]string{"bot", "editor", "30", "extra"}); err == nil || !strings.Contains(err.Error(), `unexpected argument "extra"`) {
		t.Errorf("token create, stray: %v", err)
	}
}
