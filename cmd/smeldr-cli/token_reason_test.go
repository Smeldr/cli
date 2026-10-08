package main

import "testing"

func TestParseTokenCreateArgs_Reason(t *testing.T) {
	for _, args := range [][]string{
		{"--reason", "nightly import", "bot", "editor", "30"},
		{"bot", "editor", "30", "--reason", "nightly import"},
		{"bot", "editor", "30", "--class", "job", "--reason", "nightly import"},
	} {
		name, role, ttl, _, reason, err := parseTokenCreateArgs(args)
		if err != nil || name != "bot" || role != "editor" || ttl != 30 || reason != "nightly import" {
			t.Errorf("%v: got %q %q %d reason %q err %v", args, name, role, ttl, reason, err)
		}
	}
	p := tokenCreateParams("bot", "editor", 30, "", "nightly import")
	if p["reason"] != "nightly import" {
		t.Errorf("params = %v; want the reason", p)
	}
	if _, has := tokenCreateParams("bot", "editor", 30, "", "")["reason"]; has {
		t.Error("an empty reason was sent")
	}
}

func TestParseTokenRevokeArgs(t *testing.T) {
	for _, args := range [][]string{
		{"abc", "--reason", "left"},
		{"--reason", "left", "abc"},
	} {
		id, reason, err := parseTokenRevokeArgs(args)
		if err != nil || id != "abc" || reason != "left" {
			t.Errorf("%v: got %q %q %v", args, id, reason, err)
		}
	}
	if id, reason, err := parseTokenRevokeArgs([]string{"abc"}); err != nil || id != "abc" || reason != "" {
		t.Errorf("no reason: %q %q %v", id, reason, err)
	}
	if _, _, err := parseTokenRevokeArgs(nil); err == nil {
		t.Error("missing id = nil error; want one before any request")
	}
	if _, _, err := parseTokenRevokeArgs([]string{"abc", "--reason", "x", "extra"}); err == nil {
		t.Error("extra argument = nil error")
	}
	if _, _, err := parseTokenRevokeArgs([]string{"--bogus"}); err == nil {
		t.Error("unknown flag = nil error")
	}
	if _, _, err := parseTokenRevokeArgs([]string{"abc", "--bogus"}); err == nil {
		t.Error("unknown trailing flag = nil error")
	}
	if p := tokenRevokeParams("abc", "left"); p["id"] != "abc" || p["reason"] != "left" {
		t.Errorf("params = %v", p)
	}
	if _, has := tokenRevokeParams("abc", "")["reason"]; has {
		t.Error("an empty reason was sent")
	}
}
