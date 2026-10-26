package main

import (
	"fmt"
	"strings"
	"testing"
)

// TestHandRolled_Refused (T960): the commands that parsed args by hand now
// refuse an unknown flag, a stray argument and a value flag with no value,
// with exit 1 and before any request (no server is reachable); -h exits 0.
func TestHandRolled_Refused(t *testing.T) {
	t.Setenv("SMELDR_URL", "http://127.0.0.1:1")
	t.Setenv("SMELDR_TOKEN", "test-token")
	t.Setenv("SMELDR_MCP_URL", "http://127.0.0.1:1")
	for _, tc := range []struct {
		name string
		run  func()
		code int
		want string
	}{
		{"transition, mistyped --reason", func() { runTransitionCommand([]string{"Task", "T1", "--to", "done", "--reson", "x"}) }, 1, "-reson"},
		{"transition, a third argument", func() { runTransitionCommand([]string{"Task", "T1", "T2", "--to", "done"}) }, 1, `transition: unexpected argument "T2"`},
		{"transition, --to with no value", func() { runTransitionCommand([]string{"Task", "T1", "--to"}) }, 1, "flag needs an argument: -to"},
		{"standing, stray", func() { runStandingCommand([]string{"Task", "T1", "extra"}) }, 1, `standing: unexpected argument "extra"`},
		{"approvers, mistyped flag", func() { runApproversCommand([]string{"DOM4", "--jsn"}) }, 1, "-jsn"},
		{"decision sources, mistyped flag", func() { runDecisionSourcesCommand([]string{"D148", "--jsn"}) }, 1, "-jsn"},
		{"search, mistyped flag", func() { runSearchCommand([]string{"text", "--jsn"}) }, 1, "-jsn"},
		{"search, two texts", func() { runSearchCommand([]string{"one", "two"}) }, 1, `search: unexpected argument "two"`},
		{"rests, mistyped flag", func() { runRestsCommand([]string{"Decision", "D1", "--jsn"}) }, 1, "-jsn"},
		{"premise confirm, --reason with no value", func() { runPremiseCommand([]string{"confirm", "C1", "--reason"}) }, 1, "flag needs an argument: -reason"},
		{"premise confirm, three arguments", func() { runPremiseCommand([]string{"confirm", "a", "b", "c", "--reason", "r"}) }, 1, `unexpected argument "c"`},
		{"nav list, stray", func() { runNavCommand([]string{"list", "x"}) }, 1, `nav list: unexpected argument "x"`},
		{"nav create, unknown flag", func() { runNavCommand([]string{"create", "--label", "Home", "--colour", "red"}) }, 1, "-colour"},
		{"nav update, two ids", func() { runNavCommand([]string{"update", "n1", "n2", "--label", "x"}) }, 1, `nav update: unexpected argument "n2"`},
		{"nav delete, two ids", func() { runNavCommand([]string{"delete", "n1", "n2"}) }, 1, `nav delete: unexpected argument "n2"`},
		{"redirect create, --from with no value", func() { runRedirectCommand([]string{"create", "--to", "/b", "--from"}) }, 1, "flag needs an argument: -from"},
		{"redirect delete, two paths", func() { runRedirectCommand([]string{"delete", "/a", "/b"}) }, 1, `redirect delete: unexpected argument "/b"`},
		{"webhook create, unknown flag", func() { runWebhookCommand([]string{"create", "--url", "https://x", "--event", "a"}) }, 1, "-event"},
		{"webhook deliveries, stray", func() { runWebhookCommand([]string{"deliveries", "j1"}) }, 1, `webhook deliveries: unexpected argument "j1"`},
		{"webhook delete, two ids", func() { runWebhookCommand([]string{"delete", "e1", "e2"}) }, 1, `webhook delete: unexpected argument "e2"`},
		{"webhook retry, flag", func() { runWebhookCommand([]string{"retry", "j1", "--now"}) }, 1, "-now"},
		{"social post create, unknown flag", func() {
			runSocialCommand([]string{"post", "create", "--credential", "c", "--body", "hi", "--when", "now"})
		}, 1, "-when"},
		{"social post list, stray", func() { runSocialCommand([]string{"post", "list", "x"}) }, 1, `social post list: unexpected argument "x"`},
		{"social post delete, two ids", func() { runSocialCommand([]string{"post", "delete", "p1", "p2"}) }, 1, `social post delete: unexpected argument "p2"`},
		{"social credential create, stray", func() {
			runSocialCommand([]string{"credential", "create", "x", "--platform", "x"})
		}, 1, `social credential create: unexpected argument "x"`},
		{"social schedule create, --slot with no value", func() {
			runSocialCommand([]string{"schedule", "create", "--credential", "c", "--slot"})
		}, 1, "flag needs an argument: -slot"},
		{"social schedule pause, stray", func() {
			runSocialCommand([]string{"schedule", "pause", "--credential", "c", "now"})
		}, 1, `social schedule pause: unexpected argument "now"`},
		{"social platform configure, unknown flag", func() {
			runSocialCommand([]string{"platform", "configure", "--platform", "x", "--client", "id"})
		}, 1, "-client"},
		{"block node get, two ids", func() { runNodeCommand([]string{"get", "n1", "n2"}) }, 1, `block node get: unexpected argument "n2"`},
		{"media upload, two files", func() { runMediaCommand([]string{"upload", "a.txt", "b.txt"}) }, 1, `media upload: unexpected argument "b.txt"`},
		{"media list, unknown flag", func() { runMediaCommand([]string{"list", "--kind", "image"}) }, 1, "-kind"},
		{"media delete, two ids", func() { runMediaCommand([]string{"delete", "m1", "m2"}) }, 1, `media delete: unexpected argument "m2"`},
		{"preview, a third argument", func() { runPreviewCommand([]string{"/posts", "a", "b"}) }, 1, `preview: unexpected argument "b"`},
		{"transition -h", func() { runTransitionCommand([]string{"Task", "-h"}) }, -1, ""},
		{"nav create -h", func() { runNavCommand([]string{"create", "-h"}) }, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var code int
			var stderr string
			captureStdout(t, func() { code, stderr = runExpectingExit(t, tc.run) })
			if code != tc.code || !strings.Contains(stderr, tc.want) {
				t.Errorf("exit %d, stderr %q; want exit %d with %q", code, stderr, tc.code, tc.want)
			}
		})
	}
}

// TestHandRolled_AnyOrder (T960): flags before, after or among the
// arguments reach the call.
func TestHandRolled_AnyOrder(t *testing.T) {
	tool, args, _ := mockMCP(t, map[string]any{"status": "done"}, func() {
		runTransitionCommand([]string{"--to", "done", "Task", "T1", "--reason", "shipped"})
	})
	if tool != "transition_item" || args["type_name"] != "Task" || args["slug"] != "T1" || args["to_state"] != "done" || args["reason"] != "shipped" {
		t.Errorf("transition: %q %v", tool, args)
	}
	_, args, _ = mockMCP(t, map[string]any{"hits": []any{}}, func() {
		runSearchCommand([]string{"--type", "Decision", "money", "--type", "Task", "--limit", "5"})
	})
	if args["query"] != "money" || fmt.Sprint(args["types"]) != "[Decision Task]" || fmt.Sprint(args["limit"]) != "5" {
		t.Errorf("search: %v", args)
	}
	if _, set := args["offset"]; set {
		t.Errorf("search sent an offset that was not given: %v", args)
	}
	_, args, _ = mockMCP(t, map[string]any{"id": "e1"}, func() {
		runWebhookCommand([]string{"create", "--events", "a,b", "--url", "https://hooks.example.com/x"})
	})
	if args["url"] != "https://hooks.example.com/x" || fmt.Sprint(args["events"]) != "[a b]" {
		t.Errorf("webhook create: %v", args)
	}
	_, args, _ = mockMCP(t, map[string]any{"id": "n1"}, func() {
		runNavCommand([]string{"update", "--label", "Home", "n1", "--hidden"})
	})
	if args["id"] != "n1" || args["label"] != "Home" || args["hidden"] != true {
		t.Errorf("nav update: %v", args)
	}
	if _, set := args["path"]; set {
		t.Errorf("nav update sent a path that was not given: %v", args)
	}
}
