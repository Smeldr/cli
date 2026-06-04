package main

import (
	"strings"
	"testing"
)

// ── Pure rendering ───────────────────────────────────────────────────────────

func TestRedirectListTable_RowsAndColumns(t *testing.T) {
	payload := `{"redirects":[{"from":"/old","to":"/new","code":301,"is_prefix":false},{"from":"/posts","to":"/articles","code":301,"is_prefix":true}]}`
	out, err := redirectListTable(payload)
	if err != nil {
		t.Fatalf("redirectListTable: %v", err)
	}
	for _, want := range []string{"FROM", "TO", "CODE", "PREFIX", "/old", "/new", "301", "no", "/posts", "/articles", "yes"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
}

func TestRedirectListTable_Empty(t *testing.T) {
	out, err := redirectListTable(`{"redirects":[]}`)
	if err != nil {
		t.Fatalf("redirectListTable: %v", err)
	}
	if out != "No redirects.\n" {
		t.Errorf("empty list = %q, want %q", out, "No redirects.\n")
	}
}

// ── Mock MCP harness tests ───────────────────────────────────────────────────

func TestRedirectList_ToolAndTableOutput(t *testing.T) {
	result := map[string]any{
		"redirects": []map[string]any{
			{"from": "/old", "to": "/new", "code": 301.0, "is_prefix": false},
		},
	}
	tool, _, out := mockMCP(t, result, func() {
		runRedirectCommand([]string{"list"})
	})
	if tool != "list_redirects" {
		t.Errorf("tool = %q, want list_redirects", tool)
	}
	if !strings.Contains(out, "FROM") || !strings.Contains(out, "/old") {
		t.Errorf("list output should be a table:\n%s", out)
	}
}

func TestRedirectList_JSONFlag(t *testing.T) {
	result := map[string]any{
		"redirects": []map[string]any{
			{"from": "/old", "to": "/new", "code": 301.0, "is_prefix": false},
		},
	}
	_, _, out := mockMCP(t, result, func() {
		runRedirectCommand([]string{"list", "--json"})
	})
	if !strings.Contains(out, `"redirects"`) {
		t.Errorf("--json output should contain raw JSON:\n%s", out)
	}
}

func TestRedirectCreate_ToolAndArgs(t *testing.T) {
	result := map[string]any{"from": "/old", "to": "/new", "code": 301.0, "is_prefix": false}
	tool, args, _ := mockMCP(t, result, func() {
		runRedirectCommand([]string{"create", "--from", "/old", "--to", "/new"})
	})
	if tool != "create_redirect" {
		t.Errorf("tool = %q, want create_redirect", tool)
	}
	if args["from"] != "/old" {
		t.Errorf("from = %v, want /old", args["from"])
	}
	if args["to"] != "/new" {
		t.Errorf("to = %v, want /new", args["to"])
	}
}

func TestRedirectCreate_WithCode(t *testing.T) {
	result := map[string]any{"from": "/gone", "to": "", "code": 410.0, "is_prefix": false}
	tool, args, _ := mockMCP(t, result, func() {
		runRedirectCommand([]string{"create", "--from", "/gone", "--code", "410"})
	})
	if tool != "create_redirect" {
		t.Errorf("tool = %q, want create_redirect", tool)
	}
	if args["code"] != float64(410) {
		t.Errorf("code = %v, want 410", args["code"])
	}
}

func TestRedirectCreate_PrefixFlag(t *testing.T) {
	result := map[string]any{"from": "/posts", "to": "/articles", "code": 301.0, "is_prefix": true}
	tool, args, _ := mockMCP(t, result, func() {
		runRedirectCommand([]string{"create", "--from", "/posts", "--to", "/articles", "--prefix"})
	})
	if tool != "create_redirect" {
		t.Errorf("tool = %q, want create_redirect", tool)
	}
	if args["is_prefix"] != true {
		t.Errorf("is_prefix = %v, want true", args["is_prefix"])
	}
}

func TestRedirectDelete_ToolAndArgs(t *testing.T) {
	result := map[string]any{"deleted": true, "from": "/old"}
	tool, args, _ := mockMCP(t, result, func() {
		runRedirectCommand([]string{"delete", "/old"})
	})
	if tool != "delete_redirect" {
		t.Errorf("tool = %q, want delete_redirect", tool)
	}
	if args["from"] != "/old" {
		t.Errorf("from = %v, want /old", args["from"])
	}
}
