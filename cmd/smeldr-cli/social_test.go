package main

import (
	"strings"
	"testing"
)

const sampleSocialPostListBody = `[
	{"id":"p1","platform":"mastodon","status":"draft","body":"hello world","scheduled_at":"2026-01-01T00:00:00Z"},
	{"id":"p2","platform":"x","status":"published","body":"second post"}
]`

func TestRunSocialPostList_DefaultTable(t *testing.T) {
	out := mockHTTPLogs(t, 200, sampleSocialPostListBody, func() {
		runSocialPostList([]string{})
	})
	for _, want := range []string{"ID", "PLATFORM", "STATUS", "SCHEDULED_AT", "p1", "mastodon", "draft", "p2", "x", "published"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hello world") {
		t.Errorf("default table should not include body (not in default column set):\n%s", out)
	}
}

func TestRunSocialPostList_JSONFlag(t *testing.T) {
	out := mockHTTPLogs(t, 200, sampleSocialPostListBody, func() {
		runSocialPostList([]string{"--json"})
	})
	if !strings.Contains(out, `"platform"`) {
		t.Errorf("--json output should contain raw JSON:\n%s", out)
	}
}

func TestRunSocialPostList_FieldsFlag(t *testing.T) {
	out := mockHTTPLogs(t, 200, sampleSocialPostListBody, func() {
		runSocialPostList([]string{"--fields", "id,body"})
	})
	if !strings.Contains(out, "ID") || !strings.Contains(out, "BODY") {
		t.Errorf("table should have ID and BODY columns:\n%s", out)
	}
	if strings.Contains(out, "PLATFORM") {
		t.Errorf("table should not have PLATFORM column when --fields overrides it:\n%s", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("table should show body values:\n%s", out)
	}
}

func TestRunSocialPostList_Empty(t *testing.T) {
	out := mockHTTPLogs(t, 200, `[]`, func() {
		runSocialPostList([]string{})
	})
	if out != "No posts.\n" {
		t.Errorf("empty list = %q, want %q", out, "No posts.\n")
	}
}

func TestRunSocialPostList_StatusFilterInQuery(t *testing.T) {
	req, _ := mockHTTPLogsWithRequest(t, 200, `[]`, func() {
		runSocialPostList([]string{"--status", "draft"})
	})
	if got := req.URL.Query().Get("status"); got != "draft" {
		t.Errorf("status query param = %q, want draft", got)
	}
}
