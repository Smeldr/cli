package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestParseRelationListArgs(t *testing.T) {
	la, err := parseRelationListArgs([]string{"--kind", "depends_on", "--source-type", "Task", "--target-type", "Goal",
		"--include-ended", "--as-of", "2026-10-10T20:00:00Z", "--limit", "500", "--offset", "10", "--all", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"kind": "depends_on", "source_type": "Task", "target_type": "Goal", "include_ended": true,
		"as_of": "2026-10-10T20:00:00Z", "limit": 500, "offset": 10}
	if len(la.params) != len(want) || !la.all || !la.json {
		t.Fatalf("parsed = %+v", la)
	}
	for k, v := range want {
		if la.params[k] != v {
			t.Errorf("params[%s] = %v, want %v", k, la.params[k], v)
		}
	}
	if la, err := parseRelationListArgs(nil); err != nil || len(la.params) != 0 || la.all || la.json {
		t.Errorf("no flags = %+v, %v; want empty params", la, err)
	}
	for name, args := range map[string][]string{
		"negative limit":  {"--limit", "-1"},
		"negative offset": {"--offset", "-2"},
		"stray argument":  {"extra"},
		"bad flag":        {"--nope"},
		"limit not int":   {"--limit", "x"},
	} {
		if _, err := parseRelationListArgs(args); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// fakeRelationPages answers list_relations from a fixed set of n edges, cut by
// the limit and offset it is given, recording every call's arguments.
type fakeRelationPages struct {
	n     int
	calls []map[string]any
	fail  error
}

func (f *fakeRelationPages) call(tool string, params map[string]any) (string, error) {
	if tool != "list_relations" {
		return "", fmt.Errorf("unexpected tool %s", tool)
	}
	cp := map[string]any{}
	for k, v := range params {
		cp[k] = v
	}
	f.calls = append(f.calls, cp)
	if f.fail != nil {
		return "", f.fail
	}
	limit, _ := params["limit"].(int)
	if limit == 0 {
		limit = 50
	}
	offset, _ := params["offset"].(int)
	edges := []map[string]any{}
	for i := offset; i < f.n && i < offset+limit; i++ {
		e := map[string]any{"id": fmt.Sprintf("e%d", i), "relation_kind": "depends_on", "source_type": "Task",
			"source_id": "s", "target_type": "Task", "target_id": "t", "edge_class": "asserted"}
		if i == 0 {
			e["ended"] = map[string]any{"cause": "withdrawn"}
		}
		edges = append(edges, e)
	}
	raw, _ := json.Marshal(map[string]any{"edges": edges, "total": f.n, "count": len(edges), "limit": limit,
		"offset": offset, "as_of": "2026-10-10T20:00:00.123Z"})
	return string(raw), nil
}

// --all pages to total and passes the first page's as_of back on every next page.
func TestFetchRelationPages_All(t *testing.T) {
	f := &fakeRelationPages{n: 7}
	page, err := fetchRelationPages(f.call, map[string]any{"limit": 3}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 || len(page.Edges) != 7 || page.Count != 7 || page.Total != 7 {
		t.Fatalf("calls %d, page %+v", len(f.calls), page)
	}
	if _, ok := f.calls[0]["as_of"]; ok {
		t.Error("the first page must not send as_of")
	}
	for i, c := range f.calls[1:] {
		if c["as_of"] != "2026-10-10T20:00:00.123Z" || c["offset"] != 3*(i+1) {
			t.Errorf("call %d = %v", i+1, c)
		}
	}
	for i, e := range page.Edges {
		if e["id"] != fmt.Sprintf("e%d", i) {
			t.Errorf("edge %d = %v", i, e["id"])
		}
	}
}

// Without --all there is one call; an empty set ends at once; errors come back.
func TestFetchRelationPages_OneAndErrors(t *testing.T) {
	f := &fakeRelationPages{n: 7}
	if page, err := fetchRelationPages(f.call, map[string]any{"limit": 3}, false); err != nil || len(f.calls) != 1 || page.Count != 3 || page.Total != 7 {
		t.Errorf("one page: calls %d, %+v, %v", len(f.calls), page, err)
	}
	f = &fakeRelationPages{n: 0}
	if page, err := fetchRelationPages(f.call, map[string]any{}, true); err != nil || len(f.calls) != 1 || page.Count != 0 {
		t.Errorf("empty: calls %d, %+v, %v", len(f.calls), page, err)
	}
	f = &fakeRelationPages{fail: errors.New("MCP error -32602: bad")}
	if _, err := fetchRelationPages(f.call, map[string]any{}, true); err == nil {
		t.Error("a failing call must return its error")
	}
	bad := func(string, map[string]any) (string, error) { return "not json", nil }
	if _, err := fetchRelationPages(bad, map[string]any{}, false); err == nil {
		t.Error("an undecodable result must be an error")
	}
}

func TestPrintRelationTable(t *testing.T) {
	f := &fakeRelationPages{n: 2}
	page, _ := fetchRelationPages(f.call, map[string]any{}, false)
	var buf bytes.Buffer
	printRelationTable(&buf, page)
	out := buf.String()
	for _, want := range []string{"ID", "ENDED", "e0", "Task/s", "Task/t", "withdrawn", "2 of 2 edges, as of 2026-10-10T20:00:00.123Z"} {
		if !strings.Contains(out, want) {
			t.Errorf("table lacks %q:\n%s", want, out)
		}
	}
}
