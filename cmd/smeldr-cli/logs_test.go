package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockHTTPLogs starts an httptest server that responds to any request with
// the given status code and body. It points the CLI env at the server, runs
// fn, and returns the captured stdout.
func mockHTTPLogs(t *testing.T, code int, body string, fn func()) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SMELDR_URL", srv.URL)
	t.Setenv("SMELDR_TOKEN", "test-token")
	return captureStdout(t, fn)
}

// mockHTTPLogsWithRequest is like mockHTTPLogs but also captures the incoming
// request so tests can inspect query parameters.
func mockHTTPLogsWithRequest(t *testing.T, code int, body string, fn func()) (*http.Request, string) {
	t.Helper()
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r
		w.WriteHeader(code)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SMELDR_URL", srv.URL)
	t.Setenv("SMELDR_TOKEN", "test-token")
	out := captureStdout(t, fn)
	return captured, out
}

const sampleLogsBody = `{
	"capacity": 500,
	"count": 2,
	"dropped": 0,
	"entries": [
		{"time": "2026-06-07T10:00:01Z", "level": "error", "msg": "db connection lost", "seq": 42},
		{"time": "2026-06-07T09:59:00Z", "level": "warn",  "msg": "slow query detected", "seq": 41}
	]
}`

func TestRunLogs_TableOutput(t *testing.T) {
	out := mockHTTPLogs(t, 200, sampleLogsBody, func() {
		runLogsCommand([]string{})
	})
	for _, want := range []string{"TIMESTAMP", "LEVEL", "SEQ", "MESSAGE", "ERROR", "db connection lost", "WARN", "slow query", "42", "41"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
}

func TestRunLogs_JSONFlag(t *testing.T) {
	out := mockHTTPLogs(t, 200, sampleLogsBody, func() {
		runLogsCommand([]string{"--json"})
	})
	if !strings.Contains(out, `"capacity"`) {
		t.Errorf("--json output should contain raw JSON:\n%s", out)
	}
	if !strings.Contains(out, `"entries"`) {
		t.Errorf("--json output should contain entries key:\n%s", out)
	}
}

func TestRunLogs_EmptyEntries(t *testing.T) {
	body := `{"capacity":500,"count":0,"dropped":0,"entries":[]}`
	out := mockHTTPLogs(t, 200, body, func() {
		runLogsCommand([]string{})
	})
	if !strings.Contains(out, "no log entries") {
		t.Errorf("expected 'no log entries' for empty response:\n%s", out)
	}
}

func TestRunLogs_DroppedFooter(t *testing.T) {
	body := `{
		"capacity": 500,
		"count": 1,
		"dropped": 5,
		"entries": [
			{"time": "2026-06-07T10:00:00Z", "level": "error", "msg": "panic", "seq": 100}
		]
	}`
	out := mockHTTPLogs(t, 200, body, func() {
		runLogsCommand([]string{})
	})
	if !strings.Contains(out, "5 entries dropped") {
		t.Errorf("expected dropped footer in output:\n%s", out)
	}
}

func TestRunLogs_QueryParams(t *testing.T) {
	req, _ := mockHTTPLogsWithRequest(t, 200, `{"capacity":500,"count":0,"dropped":0,"entries":[]}`, func() {
		runLogsCommand([]string{"--level", "error", "--limit", "10", "--since", "2026-01-01T00:00:00Z"})
	})
	if req == nil {
		t.Fatal("no request captured")
	}
	q := req.URL.Query()
	if q.Get("level") != "error" {
		t.Errorf("level param = %q, want error", q.Get("level"))
	}
	if q.Get("limit") != "10" {
		t.Errorf("limit param = %q, want 10", q.Get("limit"))
	}
	if q.Get("since") != "2026-01-01T00:00:00Z" {
		t.Errorf("since param = %q, want 2026-01-01T00:00:00Z", q.Get("since"))
	}
}
