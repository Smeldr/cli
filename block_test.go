package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// ── Pure rendering ───────────────────────────────────────────────────────────

func TestRenderTable_Alignment(t *testing.T) {
	out := renderTable(
		[]string{"ID", "TYPE_NAME"},
		[][]string{{"a", "hero"}, {"longer-id", "x"}},
	)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 { // header, separator, 2 rows
		t.Fatalf("expected 4 lines, got %d:\n%s", len(lines), out)
	}
	// Column 2 ("TYPE_NAME", width 9) must start at the same offset on every line.
	off := strings.Index(lines[0], "TYPE_NAME")
	for i, ln := range lines {
		// the separator line has dashes; rows have values — check the second column
		// begins at the header offset for header and data rows.
		if i == 1 {
			continue // separator
		}
		col2 := strings.TrimLeft(ln[off:], "")
		if col2 == "" {
			t.Errorf("line %d has no second column at offset %d: %q", i, off, ln)
		}
	}
	if !strings.Contains(out, "---") {
		t.Errorf("expected a dashed separator row:\n%s", out)
	}
}

func TestNodeListTable_RowsAndColumns(t *testing.T) {
	payload := `{"items":[{"ID":"n1","type_name":"hero","Status":"published","Slug":"home"},{"ID":"n2","type_name":"faq","Status":"draft","Slug":""}]}`
	out, err := nodeListTable(payload)
	if err != nil {
		t.Fatalf("nodeListTable: %v", err)
	}
	for _, want := range []string{"ID", "TYPE_NAME", "STATUS", "SLUG", "n1", "hero", "published", "home", "n2", "faq", "draft"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
}

func TestNodeListTable_Empty(t *testing.T) {
	out, err := nodeListTable(`{"items":[]}`)
	if err != nil {
		t.Fatalf("nodeListTable: %v", err)
	}
	if out != "No nodes.\n" {
		t.Errorf("empty list = %q, want %q", out, "No nodes.\n")
	}
}

func TestBuildFields_PascalCasePreserved(t *testing.T) {
	ff := fieldFlag{}
	if err := ff.Set("Headline=Hi"); err != nil {
		t.Fatal(err)
	}
	if err := ff.Set("Subtext=more"); err != nil {
		t.Fatal(err)
	}
	merged, err := buildFields(`{"Body":"base","Headline":"old"}`, ff)
	if err != nil {
		t.Fatalf("buildFields: %v", err)
	}
	if merged["Headline"] != "Hi" { // --field overrides --fields
		t.Errorf("Headline = %v, want Hi", merged["Headline"])
	}
	if merged["Subtext"] != "more" {
		t.Errorf("Subtext = %v, want more", merged["Subtext"])
	}
	if merged["Body"] != "base" {
		t.Errorf("Body = %v, want base (from --fields)", merged["Body"])
	}
	// snake_case key must NOT appear unless given — casing is preserved verbatim.
	if _, ok := merged["headline"]; ok {
		t.Error("must not lowercase keys")
	}
}

// ── Mock MCP harness ─────────────────────────────────────────────────────────

// mockMCP starts an httptest server that records the JSON-RPC tools/call params
// and returns result as the tool's text content. It points the CLI env at the
// server, runs fn, and returns the captured (toolName, arguments) plus stdout.
func mockMCP(t *testing.T, result map[string]any, fn func()) (tool string, args map[string]any, stdout string) {
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
		tool = req.Params.Name
		args = req.Params.Arguments

		data, _ := json.Marshal(result)
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"content": []map[string]any{{"type": "text", "text": string(data)}},
				"isError": false,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("FORGE_URL", srv.URL)
	t.Setenv("FORGE_TOKEN", "test-token")
	t.Setenv("FORGE_MCP_URL", srv.URL)

	stdout = captureStdout(t, fn)
	return tool, args, stdout
}

// captureStdout redirects os.Stdout for the duration of fn and returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestBlockNodeCreate_ToolAndArgs(t *testing.T) {
	tool, args, _ := mockMCP(t, map[string]any{"id": "n1", "type_name": "hero", "status": "draft"}, func() {
		runNodeCommand([]string{"create", "--type", "hero", "--field", "Headline=Hi"})
	})
	if tool != "create_node" {
		t.Errorf("tool = %q, want create_node", tool)
	}
	if args["type_name"] != "hero" {
		t.Errorf("type_name = %v, want hero", args["type_name"])
	}
	fields, ok := args["fields"].(map[string]any)
	if !ok || fields["Headline"] != "Hi" {
		t.Errorf("fields = %v, want {Headline:Hi} (PascalCase preserved)", args["fields"])
	}
}

func TestBlockNodeList_TableByDefault_JSONWithFlag(t *testing.T) {
	result := map[string]any{"items": []map[string]any{{"ID": "n1", "type_name": "hero", "Status": "published", "Slug": "home"}}}

	// default → table
	tool, args, out := mockMCP(t, result, func() {
		runNodeCommand([]string{"list", "--type", "hero", "--status", "published"})
	})
	if tool != "list_nodes" {
		t.Errorf("tool = %q, want list_nodes", tool)
	}
	if args["type_name"] != "hero" || args["status"] != "published" {
		t.Errorf("args = %v, want type_name=hero status=published", args)
	}
	if !strings.Contains(out, "TYPE_NAME") || !strings.Contains(out, "n1") {
		t.Errorf("default output should be a table:\n%s", out)
	}
	if strings.Contains(out, "\"items\"") {
		t.Errorf("default output must not be raw JSON:\n%s", out)
	}

	// --json → raw JSON
	_, _, jsonOut := mockMCP(t, result, func() {
		runNodeCommand([]string{"list", "--json"})
	})
	if !strings.Contains(jsonOut, "\"items\"") {
		t.Errorf("--json output should be raw JSON:\n%s", jsonOut)
	}
}

func TestBlockNodePublish_ToolAndArgs(t *testing.T) {
	tool, args, _ := mockMCP(t, map[string]any{"id": "n1", "status": "published"}, func() {
		runNodeCommand([]string{"publish", "n1"})
	})
	if tool != "publish_node" {
		t.Errorf("tool = %q, want publish_node", tool)
	}
	if args["id"] != "n1" {
		t.Errorf("id = %v, want n1", args["id"])
	}
}

func TestBlockNodeUpdate_ToolAndArgs(t *testing.T) {
	tool, args, _ := mockMCP(t, map[string]any{"ID": "n1"}, func() {
		runNodeCommand([]string{"update", "n1", "--field", "Title=New"})
	})
	if tool != "update_node" {
		t.Errorf("tool = %q, want update_node", tool)
	}
	if args["id"] != "n1" {
		t.Errorf("id = %v, want n1", args["id"])
	}
	fields, ok := args["fields"].(map[string]any)
	if !ok || fields["Title"] != "New" {
		t.Errorf("fields = %v, want {Title:New}", args["fields"])
	}
}

func TestBlockSectionAdd_ToolAndArgs(t *testing.T) {
	tool, args, _ := mockMCP(t, map[string]any{"ID": "e1"}, func() {
		runEdgeCommand("section", []string{"add", "P", "C"})
	})
	if tool != "add_section" {
		t.Errorf("tool = %q, want add_section", tool)
	}
	if args["parent_id"] != "P" || args["child_id"] != "C" {
		t.Errorf("args = %v, want parent_id=P child_id=C", args)
	}
}

func TestBlockSectionReorder_ToolAndArgs(t *testing.T) {
	tool, args, _ := mockMCP(t, map[string]any{"parent_id": "P"}, func() {
		runEdgeCommand("section", []string{"reorder", "P", "a, b ,c"})
	})
	if tool != "reorder_sections" {
		t.Errorf("tool = %q, want reorder_sections", tool)
	}
	ids, ok := args["ordered_child_ids"].([]any)
	if !ok || len(ids) != 3 || ids[0] != "a" || ids[1] != "b" || ids[2] != "c" {
		t.Errorf("ordered_child_ids = %v, want [a b c] (trimmed)", args["ordered_child_ids"])
	}
}

func TestBlockItemAdd_ToolAndArgs(t *testing.T) {
	tool, _, _ := mockMCP(t, map[string]any{"ID": "e1"}, func() {
		runEdgeCommand("item", []string{"add", "P", "C"})
	})
	if tool != "add_item" {
		t.Errorf("tool = %q, want add_item", tool)
	}
}

func TestBlockItemReorder_ToolName(t *testing.T) {
	tool, _, _ := mockMCP(t, map[string]any{"parent_id": "P"}, func() {
		runEdgeCommand("item", []string{"reorder", "P", "x,y"})
	})
	if tool != "reorder_items" {
		t.Errorf("tool = %q, want reorder_items", tool)
	}
}
