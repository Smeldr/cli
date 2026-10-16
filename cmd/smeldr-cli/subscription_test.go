package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseSubscriptionArgs(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.json")
	if err := os.WriteFile(file, []byte(`{"types":["Task"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		args   []string
		tool   string
		params map[string]any
	}{
		{"list", []string{"list"}, "list_subscriptions", map[string]any{}},
		{"list owner", []string{"list", "--owner", "u2"}, "list_subscriptions", map[string]any{"owner": "u2"}},
		{"get", []string{"get", "N3"}, "get_subscription", map[string]any{"subscription": "N3"}},
		{"delete", []string{"delete", "N3"}, "delete_subscription", map[string]any{"subscription": "N3"}},
		{"pause", []string{"pause", "N3"}, "update_subscription", map[string]any{"subscription": "N3", "paused": true}},
		{"resume", []string{"resume", "N3"}, "update_subscription", map[string]any{"subscription": "N3", "paused": false}},
		{"create", []string{"create", "--name", "core tasks", "--filter", `{"types":["Task"]}`, "--include-own", "--paused=false", "--owner", "u2"},
			"create_subscription", map[string]any{"name": "core tasks", "filter": map[string]any{"types": []any{"Task"}}, "include_own": true, "paused": false, "owner": "u2"}},
		{"create from file", []string{"create", "--name=x", "--filter", "@" + file}, "create_subscription", map[string]any{"name": "x", "filter": map[string]any{"types": []any{"Task"}}}},
		{"update", []string{"update", "N3", "--name", "y", "--include-own=false", "--filter", "{}"}, "update_subscription",
			map[string]any{"subscription": "N3", "name": "y", "include_own": false, "filter": map[string]any{}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool, params, err := parseSubscriptionArgs(c.args)
			if err != nil || tool != c.tool || !reflect.DeepEqual(params, c.params) {
				t.Errorf("got %q %v %v; want %q %v", tool, params, err, c.tool, c.params)
			}
		})
	}
}

func TestParseSubscriptionArgs_Errors(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"rename", "N1"},
		{"list", "extra"},
		{"list", "--nope", "x"},
		{"get"},
		{"get", "N1", "N2"},
		{"get", "N1", "--owner", "u"},
		{"create"},
		{"create", "pos", "--name", "x"},
		{"create", "--name", "x", "--filter", "[1]"},
		{"create", "--name", "x", "--filter", "@/no/such/file"},
		{"create", "--name", "x", "--paused=maybe"},
		{"create", "--name"},
		{"update", "N1"},
		{"update"},
		{"update", "N1", "--include-own=yes"},
		{"update", "N1", "--filter", "nope"},
		{"update", "N1", "--paused"},
	} {
		if _, _, err := parseSubscriptionArgs(args); err == nil {
			t.Errorf("%v: no error", args)
		}
	}
	for _, args := range [][]string{{"help"}, {"list", "--help"}} {
		if _, _, err := parseSubscriptionArgs(args); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("%v: %v, want help", args, err)
		}
	}
}

func TestParseSignalArgs(t *testing.T) {
	tool, params, err := parseSignalArgs([]string{"answer", "S9", "--text", "yes", "--ref", "Task:T4"})
	if err != nil || tool != "answer_signal" || !reflect.DeepEqual(params, map[string]any{"signal": "S9", "answer": "yes", "answer_ref": "Task:T4"}) {
		t.Errorf("answer = %q %v %v", tool, params, err)
	}
	tool, params, err = parseSignalArgs([]string{"withdraw", "--reason", "moot", "S9"})
	if err != nil || tool != "withdraw_signal" || !reflect.DeepEqual(params, map[string]any{"signal": "S9", "reason": "moot"}) {
		t.Errorf("withdraw = %q %v %v", tool, params, err)
	}
	for _, args := range [][]string{
		{}, {"ack", "S1"}, {"answer", "S1"}, {"answer", "--text", "x"}, {"answer", "S1", "--text", "x", "--nope", "y"},
		{"withdraw", "S1"}, {"withdraw"}, {"withdraw", "S1", "--reason", "x", "--text", "y"}, {"answer", "S1", "--text"},
	} {
		if _, _, err := parseSignalArgs(args); err == nil {
			t.Errorf("%v: no error", args)
		}
	}
	if _, _, err := parseSignalArgs([]string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("help: %v", err)
	}
}

func TestRunSubscriptionAndSignalCommands(t *testing.T) {
	tool, args, _ := mockMCP(t, map[string]any{"address": "N4"}, func() {
		runSubscriptionCommand([]string{"create", "--name", "mine", "--filter", `{"types":["Task"]}`})
	})
	if tool != "create_subscription" || args["name"] != "mine" {
		t.Errorf("create sent %q %v", tool, args)
	}
	tool, args, _ = mockMCP(t, map[string]any{"status": "answered"}, func() {
		runSignalCommand([]string{"answer", "S9", "--text", "yes"})
	})
	if tool != "answer_signal" || args["answer"] != "yes" {
		t.Errorf("answer sent %q %v", tool, args)
	}
	tool, _, out := mockMCP(t, map[string]any{}, func() { runSubscriptionCommand([]string{"help"}) })
	if tool != "" || !strings.Contains(out, "subscription pause <address>") {
		t.Errorf("help sent %q, printed %q", tool, out)
	}
	tool, _, out = mockMCP(t, map[string]any{}, func() { runSignalCommand([]string{"-h"}) })
	if tool != "" || !strings.Contains(out, "signal withdraw <address>") {
		t.Errorf("signal help sent %q, printed %q", tool, out)
	}
	tool, _, out = mockMCP(t, map[string]any{}, func() { runWatchCommand([]string{"--help"}) })
	if tool != "" || !strings.Contains(out, "watch --subscription <address>") {
		t.Errorf("watch help sent %q, printed %q", tool, out)
	}
}
