package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const initToken = `{"token":"tok-PERSON-0123456789","token_id":"u-person"}`

// runInitAgainst runs init in a temp directory against a mock server and
// returns the exit code, stdout, stderr and the tool calls made.
func runInitAgainst(t *testing.T, tools map[string]string, args ...string) (int, string, string, []map[string]any) {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv(initBootstrapToken.env, "boot-123")
	var calls []map[string]any
	url := initServerTools(t, tools, &calls)
	var code int
	var stderr string
	stdout := captureStdout(t, func() {
		code, stderr = runExpectingExit(t, func() { runInit(append([]string{"--url", url}, args...)) })
	})
	return code, stdout, stderr, calls
}

// granted lists each grant_role call as role/token_id/reason, in order.
func granted(calls []map[string]any) []string {
	var roles []string
	for _, c := range calls {
		if c["tool"] == "grant_role" {
			a := c["args"].(map[string]any)
			roles = append(roles, fmt.Sprint(a["role"], "/", a["token_id"], "/", a["reason"]))
		}
	}
	return roles
}

// TestInit_Founder (T970): on a fresh governed server init creates a human
// token, grants it admin and approver with D142's founder reason, and
// verifies it can act as admin.
func TestInit_Founder(t *testing.T) {
	code, stdout, stderr, calls := runInitAgainst(t, map[string]string{"create_token": initToken})
	if code != -1 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	var create map[string]any
	for _, c := range calls {
		if c["tool"] == "create_token" {
			create = c["args"].(map[string]any)
		}
	}
	if create["actor_class"] != "human" {
		t.Errorf("create_token args = %v, want actor_class human", create)
	}
	want := []string{"admin/u-person/founder of the organization (D142)", "approver/u-person/founder of the organization (D142)"}
	if got := granted(calls); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("grants = %v, want %v", got, want)
	}
	for _, line := range []string{"class human, admin", "grants: admin, approver (founder of the organization, D142): this token can ratify Decisions", "the new token acts as admin"} {
		if !strings.Contains(stdout, line) {
			t.Errorf("stdout misses %q:\n%s", line, stdout)
		}
	}
	if last := calls[len(calls)-1]["tool"]; last != "list_tokens" {
		t.Errorf("last call = %v, want the list_tokens verification", last)
	}
}

// TestInit_GrantCases (T970): --no-approver and an existing global approver
// grant admin only, and say so; a scoped or expired approver does not count;
// a server without governance grants nothing; a failing grant names the
// token, never a token value.
func TestInit_GrantCases(t *testing.T) {
	existing := `{"grants":[{"RoleName":"approver","ScopeAnchorID":"","ScopeStatic":[],"ExpiresAt":null}]}`
	scoped := `{"grants":[{"RoleName":"approver","ScopeAnchorID":"are-1","ScopeStatic":[],"ExpiresAt":null}]}`
	expired := `{"grants":[{"RoleName":"approver","ScopeAnchorID":"","ScopeStatic":[],"ExpiresAt":"2001-01-01T00:00:00Z"}]}`
	for _, tc := range []struct {
		name   string
		tools  map[string]string
		args   []string
		code   int
		grants string
		out    string
		errOut string
	}{
		{"no approver flag", map[string]string{"create_token": initToken}, []string{"--no-approver"}, -1, "admin", "--no-approver: this token cannot ratify", ""},
		{"a global approver exists", map[string]string{"create_token": initToken, "list_grants": existing}, nil, -1, "admin", "a global approver already exists", ""},
		{"only a scoped approver exists", map[string]string{"create_token": initToken, "list_grants": scoped}, nil, -1, "admin,approver", "founder of the organization", ""},
		{"only an expired approver exists", map[string]string{"create_token": initToken, "list_grants": expired}, nil, -1, "admin,approver", "founder of the organization", ""},
		{"no governance", map[string]string{"create_token": initToken, "list_grants": "ERROR unknown tool: list_grants"}, nil, -1, "", "no governance on this server", ""},
		{"grant fails", map[string]string{"create_token": initToken, "grant_role": "ERROR forbidden"}, nil, 1, "admin", "", `grant_role admin for token "operator" failed`},
		{"verification fails", map[string]string{"create_token": initToken, "list_tokens": "ERROR forbidden"}, nil, -1, "admin,approver", "", "the new token cannot act as admin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr, calls := runInitAgainst(t, tc.tools, tc.args...)
			if code != tc.code {
				t.Fatalf("exit %d, want %d; stderr %q", code, tc.code, stderr)
			}
			var roles []string
			for _, g := range granted(calls) {
				roles = append(roles, strings.SplitN(g, "/", 2)[0])
			}
			if strings.Join(roles, ",") != tc.grants {
				t.Errorf("grants = %v, want %s", roles, tc.grants)
			}
			if tc.out != "" && !strings.Contains(stdout, tc.out) {
				t.Errorf("stdout misses %q:\n%s", tc.out, stdout)
			}
			if tc.errOut != "" && !strings.Contains(stderr, tc.errOut) {
				t.Errorf("stderr misses %q: %q", tc.errOut, stderr)
			}
			for _, part := range []string{"tok-PERSON", "0123456789", "boot-123"} {
				if strings.Contains(stderr, part) || strings.Contains(stdout, part) {
					t.Errorf("output shows %q", part)
				}
			}
		})
	}
}

// TestInit_ExistingEnvFileRefusedFirst (T970): an existing env file without
// --force is refused before create_token, so no orphaned token is made.
func TestInit_ExistingEnvFileRefusedFirst(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(initBootstrapToken.env, "boot-123")
	if err := os.WriteFile(".smeldr-cli.env", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls []map[string]any
	url := initServerTools(t, map[string]string{"create_token": initToken}, &calls)
	var code int
	captureStdout(t, func() { code, _ = runExpectingExit(t, func() { runInit([]string{"--url", url}) }) })
	if code != 1 || len(calls) != 0 {
		t.Errorf("exit %d after %d tool calls, want 1 after none", code, len(calls))
	}
}
