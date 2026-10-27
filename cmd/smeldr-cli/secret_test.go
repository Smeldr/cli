package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
)

const testSecret = "s3cret-value-never-shown"

// unsetEnv removes name for the test and restores it afterwards.
func unsetEnv(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	os.Unsetenv(name) //nolint:errcheck
}

// TestSecretInput (T961): a secret comes from exactly one of the env var,
// stdin and the deprecated flag; the flag warns; no error or warning ever
// shows the secret.
func TestSecretInput(t *testing.T) {
	src := socialClientSecret
	for _, tc := range []struct {
		name    string
		args    []string
		env     *string
		in      string
		want    string
		wantErr string
		warns   bool
	}{
		{"env", nil, ptr(testSecret), "", testSecret, "", false},
		{"stdin", []string{"--client-secret-stdin"}, nil, testSecret + "\n", testSecret, "", false},
		{"stdin without a newline", []string{"--client-secret-stdin"}, nil, testSecret, testSecret, "", false},
		{"flag", []string{"--client-secret", testSecret}, nil, "", testSecret, "", true},
		{"flag and env", []string{"--client-secret", testSecret}, ptr(testSecret), "", "", "give the secret one way, not --client-secret and $SMELDR_SOCIAL_CLIENT_SECRET", false},
		{"stdin and env", []string{"--client-secret-stdin"}, ptr(testSecret), testSecret + "\n", "", "not --client-secret-stdin and $SMELDR_SOCIAL_CLIENT_SECRET", false},
		{"none", nil, nil, "", "", "requires the app's client secret (set $SMELDR_SOCIAL_CLIENT_SECRET, or use --client-secret-stdin)", false},
		{"empty env", nil, ptr("  "), "", "", "$SMELDR_SOCIAL_CLIENT_SECRET is empty", false},
		{"empty flag", []string{"--client-secret", ""}, nil, "", "", "--client-secret is empty", false},
		{"empty stdin", []string{"--client-secret-stdin"}, nil, "", "", "nothing on stdin", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != nil {
				t.Setenv(src.env, *tc.env)
			} else {
				unsetEnv(t, src.env)
			}
			old := stdin
			stdin = strings.NewReader(tc.in)
			t.Cleanup(func() { stdin = old })
			fs := newCommandFlags("t")
			value, fromStdin := src.secretFlags(fs, "the secret")
			if err := fs.Parse(tc.args); err != nil {
				t.Fatal(err)
			}
			var got string
			var err error
			_, stderr := runExpectingExit(t, func() { got, err = src.read(fs, *value, *fromStdin) })
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				if strings.Contains(err.Error(), testSecret) {
					t.Errorf("the error shows the secret: %v", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v", got, err)
			}
			if warned := strings.Contains(stderr, "warning: --client-secret puts the secret in shell history"); warned != tc.warns {
				t.Errorf("warned = %v, stderr %q", warned, stderr)
			}
			if strings.Contains(stderr, testSecret) {
				t.Errorf("stderr shows the secret: %q", stderr)
			}
		})
	}
}

func ptr(s string) *string { return &s }

// TestSocialPlatformConfigure_SecretFromEnv (T961): the env var reaches
// client_secret with no warning; the flag still works, with one.
func TestSocialPlatformConfigure_SecretFromEnv(t *testing.T) {
	base := []string{"platform", "configure", "--platform", "mastodon", "--client-id", "id", "--redirect-url", "https://x/cb"}
	t.Setenv(socialClientSecret.env, testSecret)
	var stderr string
	_, args, _ := mockMCP(t, map[string]any{"ok": true}, func() {
		_, stderr = runExpectingExit(t, func() { runSocialCommand(base) })
	})
	if args["client_secret"] != testSecret || strings.Contains(stderr, "warning") {
		t.Errorf("env: args %v, stderr %q", args, stderr)
	}
	unsetEnv(t, socialClientSecret.env)
	_, args, _ = mockMCP(t, map[string]any{"ok": true}, func() {
		_, stderr = runExpectingExit(t, func() { runSocialCommand(append(base, "--client-secret", testSecret)) })
	})
	if args["client_secret"] != testSecret || !strings.Contains(stderr, "warning: --client-secret") || strings.Contains(stderr, testSecret) {
		t.Errorf("flag: args %v, stderr %q", args, stderr)
	}
}

// initServer answers /_health and the MCP tools init calls: create_token
// with text, list_grants with no grants, and the rest with {}.
func initServer(t *testing.T, text string) string {
	t.Helper()
	return initServerTools(t, map[string]string{"create_token": text}, nil)
}

// initServerTools answers each tool with tools[name] (default {}, and no
// grants for list_grants; a value starting "ERROR " is a JSON-RPC error)
// and records every call in calls.
func initServerTools(t *testing.T, tools map[string]string, calls *[]map[string]any) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_health" {
			_, _ = io.WriteString(w, `{"status":"ok"}`)
			return
		}
		var req struct {
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if calls != nil {
			*calls = append(*calls, map[string]any{"tool": req.Params.Name, "args": req.Params.Arguments})
		}
		text, ok := tools[req.Params.Name]
		if !ok {
			text = "{}"
			if req.Params.Name == "list_grants" {
				text = `{"grants":[]}`
			}
		}
		if strings.HasPrefix(text, "ERROR ") {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "error": map[string]any{"code": -32602, "message": strings.TrimPrefix(text, "ERROR ")}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}}, "isError": false}})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestInit_Secrets (T961): init takes the bootstrap token from the env var,
// writes the env file 0600 even over a wider existing file, and never shows
// any part of a new token in an error.
func TestInit_Secrets(t *testing.T) {
	t.Chdir(t.TempDir())
	const newToken = "tok-NEWADMINTOKEN-0123456789"
	url := initServer(t, `{"token":"`+newToken+`","token_id":"u-1"}`)
	t.Setenv(initBootstrapToken.env, "boot-123")
	if err := os.WriteFile(".smeldr-cli.env", []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var code int
	var stderr string
	captureStdout(t, func() { code, stderr = runExpectingExit(t, func() { runInit([]string{"--url", url, "--force"}) }) })
	if code != -1 || strings.Contains(stderr, "warning") {
		t.Fatalf("init: exit %d, stderr %q", code, stderr)
	}
	b, err := os.ReadFile(".smeldr-cli.env")
	if err != nil || !strings.Contains(string(b), "SMELDR_TOKEN="+newToken) {
		t.Fatalf("env file: %q %v", b, err)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(".smeldr-cli.env"); fi.Mode().Perm() != 0o600 {
			t.Errorf("mode %04o after --force over 0644, want 0600", fi.Mode().Perm())
		}
	}

	for _, body := range []string{`not json: ` + newToken, `{"name":"operator","note":"` + newToken + `"}`} {
		url := initServer(t, body)
		captureStdout(t, func() { code, stderr = runExpectingExit(t, func() { runInit([]string{"--url", url, "--force"}) }) })
		if code != 1 {
			t.Errorf("%q: exit %d", body, code)
		}
		for _, part := range []string{newToken, "NEWADMINTOKEN", "0123456789", "raw:"} {
			if strings.Contains(stderr, part) {
				t.Errorf("%q: stderr shows %q: %s", body, part, stderr)
			}
		}
	}
}

// TestWarnReadableByOthers (T961): a group- or world-readable env file is
// warned about on Unix, with the mode and the fix and never the content.
func TestWarnReadableByOthers(t *testing.T) {
	_, wide := runExpectingExit(t, func() { warnReadableByOthers(".smeldr-cli.env", 0o644) })
	_, tight := runExpectingExit(t, func() { warnReadableByOthers(".smeldr-cli.env", 0o600) })
	if tight != "" {
		t.Errorf("0600 warned: %q", tight)
	}
	if runtime.GOOS == "windows" {
		if wide != "" {
			t.Errorf("Windows warned: %q", wide)
		}
		return
	}
	if !strings.Contains(wide, "readable by others (mode 0644); run chmod 600 .smeldr-cli.env") {
		t.Errorf("0644: %q", wide)
	}
}

// TestTokenCreateHelp (T961): the help says the token is printed once.
func TestTokenCreateHelp(t *testing.T) {
	code, stderr := runExpectingExit(t, func() { runTokenCommand([]string{"create", "-h"}) })
	if code != 0 || !strings.Contains(stderr, "printed once, to stdout") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}
