package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testCommit = "0123456789abcdef0123456789abcdef01234567"

func TestDecisionSources_List(t *testing.T) {
	result := map[string]any{"address": "D148", "sources": []any{
		map[string]any{"repository": "acme/app", "commit": testCommit, "path": "docs/money.md", "anchor": "## Cents", "role": "source"},
	}}
	tool, args, out := mockMCP(t, result, func() { runDecisionCommand([]string{"sources", "D148"}) })
	if tool != "get_decision" || args["slug"] != "D148" {
		t.Errorf("tool %q, args %v", tool, args)
	}
	for _, want := range []string{"ROLE", "source", "acme/app", "0123456789ab", "docs/money.md", "## Cents"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	_, _, out = mockMCP(t, map[string]any{"sources": []any{}}, func() { runDecisionCommand([]string{"sources", "D9"}) })
	if !strings.Contains(out, "D9 has no sources") {
		t.Errorf("empty:\n%s", out)
	}
	_, _, out = mockMCP(t, result, func() { runDecisionCommand([]string{"sources", "D148", "--json"}) })
	if !strings.Contains(out, `"path": "docs/money.md"`) {
		t.Errorf("--json:\n%s", out)
	}
	_, _, out = mockMCP(t, result, func() { runDecisionCommand([]string{"sources", "--help"}) })
	if !strings.Contains(out, "pinned external sources") {
		t.Errorf("--help:\n%s", out)
	}
}

func TestDecisionSources_Set(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sources.json")
	body := fmt.Sprintf(`[{"repository":"acme/app","commit":%q,"path":"docs/a.md","role":"evidence"}]`, testCommit)
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	tool, args, out := mockMCP(t, map[string]any{}, func() { runDecisionCommand([]string{"sources", "D148", "--set", file, "--rev", "2"}) })
	list, _ := args["sources"].([]any)
	if tool != "update_decision" || args["slug"] != "D148" || len(list) != 1 || !strings.Contains(out, "1 sources set") {
		t.Errorf("tool %q, args %v, out %s", tool, args, out)
	}
	stdin = strings.NewReader(body)
	old := stdin
	t.Cleanup(func() { stdin = old })
	_, args, _ = mockMCP(t, map[string]any{}, func() { runDecisionCommand([]string{"sources", "D148", "--set", "-", "--rev", "2"}) })
	if list, _ := args["sources"].([]any); len(list) != 1 {
		t.Errorf("stdin: %v", args)
	}
	_, args, out = mockMCP(t, map[string]any{}, func() { runDecisionCommand([]string{"sources", "D148", "--clear", "--rev", "2"}) })
	if list, ok := args["sources"].([]any); !ok || len(list) != 0 || !strings.Contains(out, "0 sources set") {
		t.Errorf("--clear: %v %s", args, out)
	}
}

// TestDecision_ContentVerbsUseDecisionsRoute: smeldr-cli decision create,
// get and list reach core's /decisions route, where Decisions are mounted
// (T1122); before, they went to /decision and answered 404.
func TestDecision_ContentVerbsUseDecisionsRoute(t *testing.T) {
	type call struct{ method, path string }
	var calls []call
	var created map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, call{r.Method, r.URL.Path})
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(&created)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"address":"D200","Status":"proposed"}`))
		case r.URL.Path == "/decisions":
			_, _ = w.Write([]byte(`[]`))
		default:
			_, _ = w.Write([]byte(`{"address":"D12"}`))
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SMELDR_URL", srv.URL)
	t.Setenv("SMELDR_TOKEN", "test-token")

	file := filepath.Join(t.TempDir(), "decision.md")
	md := "---\ntitle: Money is stored in cents\nscope: billing\n---\nWhy: rounding.\n"
	if err := os.WriteFile(file, []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
	captureStdout(t, func() { runDecisionCommand([]string{"create", "--from", file}) })
	captureStdout(t, func() { runDecisionCommand([]string{"get", "D12"}) })
	captureStdout(t, func() { runDecisionCommand([]string{"list"}) })

	want := []call{{http.MethodPost, "/decisions"}, {http.MethodGet, "/decisions/D12"}, {http.MethodGet, "/decisions"}}
	if fmt.Sprint(calls) != fmt.Sprint(want) {
		t.Errorf("calls = %v, want %v", calls, want)
	}
	if created["title"] != "Money is stored in cents" || created["scope"] != "billing" || !strings.Contains(fmt.Sprint(created["Body"]), "Why: rounding.") {
		t.Errorf("create sent %v", created)
	}
}
