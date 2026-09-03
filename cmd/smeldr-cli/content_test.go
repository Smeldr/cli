package main

import (
	"strings"
	"testing"
)

const sampleContentListBody = `[
	{"ID":"n1","Slug":"post-one","Status":"published","CreatedAt":"2026-01-01T00:00:00Z","UpdatedAt":"2026-01-02T00:00:00Z","Title":"Post One"},
	{"ID":"n2","Slug":"post-two","Status":"draft","CreatedAt":"2026-01-03T00:00:00Z","UpdatedAt":"2026-01-03T00:00:00Z","Title":"Post Two"}
]`

func TestRunList_DefaultTable(t *testing.T) {
	out := mockHTTPLogs(t, 200, sampleContentListBody, func() {
		runList("posts", []string{})
	})
	for _, want := range []string{"SLUG", "STATUS", "CREATEDAT", "UPDATEDAT", "post-one", "published", "post-two", "draft"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Title") {
		t.Errorf("default table should not include Title (not in default column set):\n%s", out)
	}
}

func TestRunList_JSONFlag(t *testing.T) {
	out := mockHTTPLogs(t, 200, sampleContentListBody, func() {
		runList("posts", []string{"--json"})
	})
	if !strings.Contains(out, `"Slug": "post-one"`) {
		t.Errorf("--json output should contain raw JSON:\n%s", out)
	}
}

func TestRunList_FieldsFlag(t *testing.T) {
	out := mockHTTPLogs(t, 200, sampleContentListBody, func() {
		runList("posts", []string{"--fields", "slug,title"})
	})
	if !strings.Contains(out, "SLUG") || !strings.Contains(out, "TITLE") {
		t.Errorf("table should have SLUG and TITLE columns:\n%s", out)
	}
	if strings.Contains(out, "STATUS") {
		t.Errorf("table should not have STATUS column when --fields overrides it:\n%s", out)
	}
	if !strings.Contains(out, "Post One") || !strings.Contains(out, "Post Two") {
		t.Errorf("table should show Title values:\n%s", out)
	}
}

func TestRunList_Empty(t *testing.T) {
	out := mockHTTPLogs(t, 200, `[]`, func() {
		runList("posts", []string{})
	})
	if out != "No items.\n" {
		t.Errorf("empty list = %q, want %q", out, "No items.\n")
	}
}

func TestRunList_StatusFilterAppliesBeforeTable(t *testing.T) {
	out := mockHTTPLogs(t, 200, sampleContentListBody, func() {
		runList("posts", []string{"--status", "draft"})
	})
	if strings.Contains(out, "post-one") {
		t.Errorf("--status draft should exclude published post-one:\n%s", out)
	}
	if !strings.Contains(out, "post-two") {
		t.Errorf("--status draft should include draft post-two:\n%s", out)
	}
}
