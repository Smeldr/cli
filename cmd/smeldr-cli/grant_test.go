package main

import (
	"errors"
	"flag"
	"reflect"
	"strings"
	"testing"
)

func TestParseGrantArgs(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		tool   string
		params map[string]any
	}{
		{"grant", []string{"tok-1", "steward"}, "grant_role", map[string]any{"token_id": "tok-1", "role": "steward"}},
		{"grant, flags after", []string{"tok-1", "steward", "--scope", "post:*", "--scope", "doc:a", "--anchor", "g1", "--reason", "owns docs"},
			"grant_role", map[string]any{"token_id": "tok-1", "role": "steward", "scope_static": []any{"post:*", "doc:a"}, "scope_anchor_id": "g1", "reason": "owns docs"}},
		{"grant, flags before", []string{"--reason", "owns docs", "tok-1", "steward"},
			"grant_role", map[string]any{"token_id": "tok-1", "role": "steward", "reason": "owns docs"}},
		{"revoke", []string{"revoke", "g-1"}, "revoke_grant", map[string]any{"id": "g-1"}},
		{"revoke with reason", []string{"revoke", "g-1", "--reason", "rotation over"}, "revoke_grant", map[string]any{"id": "g-1", "reason": "rotation over"}},
		{"revoke, flag before", []string{"revoke", "--reason", "rotation over", "g-1"}, "revoke_grant", map[string]any{"id": "g-1", "reason": "rotation over"}},
		{"list all", []string{"list"}, "list_grants", map[string]any{}},
		{"list one", []string{"list", "tok-1"}, "list_grants", map[string]any{"token_id": "tok-1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool, params, err := parseGrantArgs(c.args)
			if err != nil || tool != c.tool || !reflect.DeepEqual(params, c.params) {
				t.Errorf("got %q %v %v; want %q %v", tool, params, err, c.tool, c.params)
			}
		})
	}
}

func TestParseGrantArgs_Errors(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"tok-1"},
		{"tok-1", "steward", "extra"},
		{"tok-1", "steward", "--bogus"},
		{"--bogus", "tok-1", "steward"},
		{"revoke"},
		{"revoke", "--bogus"},
		{"revoke", "g-1", "--reason", "x", "extra"},
		{"list", "a", "b"},
		{"list", "--bogus"},
	} {
		if _, _, err := parseGrantArgs(args); err == nil || errors.Is(err, flag.ErrHelp) {
			t.Errorf("%v: err = %v; want an error before any request", args, err)
		}
	}
}

// help, -h and --help print usage and send nothing: parseGrantArgs returns
// flag.ErrHelp, which runGrantCommand turns into the usage text.
func TestParseGrantArgs_Help(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}, {"list", "-h"}, {"revoke", "-h"}, {"tok", "role", "-h"}} {
		if _, _, err := parseGrantArgs(args); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("%v: err = %v; want flag.ErrHelp", args, err)
		}
	}
}

func TestStringsFlag(t *testing.T) {
	var s stringsFlag
	_ = s.Set("a")
	_ = s.Set("b")
	if s.String() != "a,b" {
		t.Errorf("String() = %q", s.String())
	}
}

// Each command sends the right tool and arguments; help prints usage and
// sends no request.
func TestRunGrantCommand(t *testing.T) {
	tool, args, _ := mockMCP(t, map[string]any{"id": "g-1"}, func() {
		runGrantCommand([]string{"tok-1", "steward", "--reason", "owns docs"})
	})
	if tool != "grant_role" || args["token_id"] != "tok-1" || args["reason"] != "owns docs" {
		t.Errorf("grant sent %q %v", tool, args)
	}
	tool, args, _ = mockMCP(t, map[string]any{"revoked": true}, func() {
		runGrantCommand([]string{"revoke", "g-1"})
	})
	if tool != "revoke_grant" || args["id"] != "g-1" {
		t.Errorf("revoke sent %q %v", tool, args)
	}
	tool, _, _ = mockMCP(t, map[string]any{"grants": []any{}}, func() {
		runGrantCommand([]string{"list"})
	})
	if tool != "list_grants" {
		t.Errorf("list sent %q", tool)
	}
	for _, h := range []string{"help", "-h"} {
		tool, _, out := mockMCP(t, map[string]any{}, func() {
			runGrantCommand([]string{h})
		})
		if tool != "" {
			t.Errorf("grant %s sent a request (%q)", h, tool)
		}
		if !strings.Contains(out, "grant revoke <grant-id>") {
			t.Errorf("grant %s printed %q; want the usage", h, out)
		}
	}
}

func TestParseGrantArgs_ExpiresInDays(t *testing.T) {
	for _, args := range [][]string{
		{"tok-1", "steward", "--expires-in-days", "14"},
		{"--expires-in-days", "14", "tok-1", "steward"},
	} {
		tool, p, err := parseGrantArgs(args)
		if err != nil || tool != "grant_role" || p["expires_in_days"] != float64(14) {
			t.Errorf("%v: %q %v %v; want expires_in_days 14", args, tool, p, err)
		}
	}
	if _, p, _ := parseGrantArgs([]string{"tok-1", "steward", "--expires-in-days", "0.5"}); p["expires_in_days"] != 0.5 {
		t.Errorf("fraction = %v; want 0.5", p["expires_in_days"])
	}
	if _, p, err := parseGrantArgs([]string{"tok-1", "steward", "--expires-in-days", "36500"}); err != nil || p["expires_in_days"] != float64(36500) {
		t.Errorf("36500 days = %v %v; want accepted", p, err)
	}
	for _, v := range []string{"0", "-1", "two", "36501", "1e6"} {
		if _, _, err := parseGrantArgs([]string{"tok-1", "steward", "--expires-in-days", v}); err == nil {
			t.Errorf("--expires-in-days %s: nil error; want one before any request", v)
		}
	}
	if _, p, _ := parseGrantArgs([]string{"tok-1", "steward"}); p["expires_in_days"] != nil {
		t.Error("expires_in_days sent without the flag")
	}
	tool, args, _ := mockMCP(t, map[string]any{"id": "g-1"}, func() {
		runGrantCommand([]string{"tok-1", "steward", "--expires-in-days", "7"})
	})
	if tool != "grant_role" || args["expires_in_days"] != float64(7) {
		t.Errorf("sent %q %v; want expires_in_days 7", tool, args)
	}
}
