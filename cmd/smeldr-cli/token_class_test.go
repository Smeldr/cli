package main

import "testing"

func TestParseTokenCreateArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		class   string
	}{
		{"plain", []string{"ci", "author", "30"}, false, ""},
		{"flag after", []string{"bot", "editor", "90", "--class", "agent"}, false, "agent"},
		{"flag before", []string{"--class", "job", "cron", "editor", "90"}, false, "job"},
		{"flag with equals", []string{"peter", "admin", "30", "--class=human"}, false, "human"},
		{"invalid class", []string{"x", "editor", "30", "--class", "robot"}, true, ""},
		{"extra argument", []string{"x", "editor", "30", "stray"}, true, ""},
		{"too few", []string{"x", "editor"}, true, ""},
		{"bad ttl", []string{"x", "editor", "soon"}, true, ""},
		{"zero ttl", []string{"x", "editor", "0"}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, role, ttl, class, _, _, err := parseTokenCreateArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if class != tt.class || name == "" || role == "" || ttl <= 0 {
				t.Errorf("got %q %q %d class %q, want class %q", name, role, ttl, class, tt.class)
			}
		})
	}
}

func TestTokenCreateParams(t *testing.T) {
	p := tokenCreateParams("bot", "editor", 30, "agent", "", "")
	if p["actor_class"] != "agent" || p["name"] != "bot" || p["role"] != "editor" || p["expires_in_days"] != 30 {
		t.Errorf("params = %v", p)
	}
	if _, has := tokenCreateParams("ci", "author", 30, "", "", "")["actor_class"]; has {
		t.Error("actor_class must be left out when no class was given")
	}
}

// --identity is sent as identity, only when given (D165).
func TestTokenCreateIdentity(t *testing.T) {
	_, _, _, _, _, identity, err := parseTokenCreateArgs([]string{"bot-2", "editor", "30", "--identity", "IDN4"})
	if err != nil || identity != "IDN4" {
		t.Fatalf("parse --identity = %q, %v", identity, err)
	}
	if got := tokenCreateParams("bot-2", "editor", 30, "", "", "IDN4")["identity"]; got != "IDN4" {
		t.Errorf("identity param = %v", got)
	}
	if _, has := tokenCreateParams("bot", "editor", 30, "", "", "")["identity"]; has {
		t.Error("no --identity must send no identity")
	}
}
