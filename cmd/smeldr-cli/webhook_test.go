package main

import (
	"strings"
	"testing"
)

// webhook retry's help says which jobs it sends (core T1266), and a refusal
// from the server is printed with exit 1; a retried job prints the result.
func TestWebhookRetry(t *testing.T) {
	help := captureStdout(t, printWebhookHelp)
	if !strings.Contains(help, "send a dead or pending job now (a delivered job is refused)") {
		t.Errorf("webhook help:\n%s", help)
	}

	msg := "Conflict: webhook job j1 was already delivered, so it is not sent again; nothing changed"
	s := &revServer{
		results: map[string]any{"retry_webhook": map[string]any{"retried": true, "job_id": "j2"}},
		errors:  map[string][2]any{},
	}
	s.start(t)
	out := captureStdout(t, func() { runWebhookCommand([]string{"retry", "j2"}) })
	if !strings.Contains(out, `"retried": true`) {
		t.Errorf("retry of a dead job:\n%s", out)
	}
	s.errors["retry_webhook"] = [2]any{-32001, msg}
	code, stderr := runExpectingExit(t, func() { runWebhookCommand([]string{"retry", "j1"}) })
	if code != 1 || !strings.Contains(stderr, msg) {
		t.Errorf("retry of a delivered job: exit %d\n%s", code, stderr)
	}
}
