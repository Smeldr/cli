package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// core T1187 (D189): decision supersede, retire and repair.

func TestDecisionSupersede_Forms(t *testing.T) {
	tool, args, _ := mockMCP(t, map[string]any{"decision": "D186", "replacement": "D188", "created": false}, func() {
		runDecisionCommand([]string{"supersede", "D186", "--by", "D188", "--reason", "new wording", "--rev", "3"})
	})
	if tool != "supersede_decision" || args["decision"] != "D186" || args["replacement"] != "D188" || args["reason"] != "new wording" || args["rev"] != float64(3) || args["new"] != nil {
		t.Errorf("--by: tool %q, args %v", tool, args)
	}
	file := filepath.Join(t.TempDir(), "new.md")
	if err := os.WriteFile(file, []byte("---\ntitle: The clearer rule\nscope: core\n---\nEvery write names its place.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool, args, _ = mockMCP(t, map[string]any{"created": true}, func() {
		runDecisionCommand([]string{"supersede", "D186", "--new", "--from", file, "--reason", "clearer", "--rev", "3"})
	})
	n, _ := args["new"].(map[string]any)
	if tool != "supersede_decision" || n["title"] != "The clearer rule" || n["scope"] != "core" || !strings.Contains(n["body"].(string), "Every write names its place.") || args["replacement"] != nil {
		t.Errorf("--new: tool %q, args %v", tool, args)
	}
}

func TestDecisionSupersede_RefusedLocally(t *testing.T) {
	for name, a := range map[string][]string{
		"no reason":        {"supersede", "D1", "--by", "D2"},
		"both forms":       {"supersede", "D1", "--by", "D2", "--new", "--from", "x.md", "--reason", "r"},
		"neither form":     {"supersede", "D1", "--reason", "r"},
		"new no file":      {"supersede", "D1", "--new", "--reason", "r"},
		"retire no reason": {"retire", "D1"},
		"repair both":      {"repair", "D1", "--by", "D2", "--retire", "--reason", "r"},
		"repair neither":   {"repair", "D1", "--reason", "r"},
	} {
		code, stderr := runExpectingExit(t, func() { runDecisionCommand(a) })
		if code != 1 || stderr == "" {
			t.Errorf("%s: exit %d, stderr %q", name, code, stderr)
		}
	}
}

func TestDecisionRetireAndRepair(t *testing.T) {
	tool, args, _ := mockMCP(t, map[string]any{"status": "retired"}, func() {
		runDecisionCommand([]string{"retire", "D172", "--reason", "a throwaway", "--rev", "1"})
	})
	if tool != "retire_decision" || args["decision"] != "D172" || args["reason"] != "a throwaway" {
		t.Errorf("retire: tool %q, args %v", tool, args)
	}
	tool, args, _ = mockMCP(t, map[string]any{"relation": "r1"}, func() {
		runDecisionCommand([]string{"repair", "D186", "--by", "D188", "--reason", "D188 replaced it", "--rev", "2"})
	})
	if tool != "repair_superseded" || args["replacement"] != "D188" || args["retire"] != nil {
		t.Errorf("repair --by: tool %q, args %v", tool, args)
	}
	tool, args, _ = mockMCP(t, map[string]any{"status": "retired"}, func() {
		runDecisionCommand([]string{"repair", "D172", "--retire", "--reason", "a throwaway", "--rev", "2"})
	})
	if tool != "repair_superseded" || args["retire"] != true || args["replacement"] != nil {
		t.Errorf("repair --retire: tool %q, args %v", tool, args)
	}
	for _, sub := range []string{"supersede", "retire", "repair"} {
		_, _, out := mockMCP(t, map[string]any{}, func() { runDecisionCommand([]string{sub, "--help"}) })
		if !strings.Contains(out, "D189") {
			t.Errorf("%s --help:\n%s", sub, out)
		}
	}
	_, _, out := mockMCP(t, map[string]any{}, func() { runDecisionCommand([]string{"--help"}) })
	for _, want := range []string{"decision supersede", "decision retire", "decision repair"} {
		if !strings.Contains(out, want) {
			t.Errorf("decision --help lacks %q", want)
		}
	}
}

// TestTransition_BareSupersedeHint: the server's warning is printed with its
// date, and the cli names the commands instead; it writes no date of its own.
func TestTransition_BareSupersedeHint(t *testing.T) {
	result := map[string]any{"status": "superseded", "warnings": []any{map[string]any{
		"code": "decision.supersede_without_replacement", "message": "D9 moved to superseded with no replacement recorded; from 2099-01-01 this is refused"}}}
	code, stderr := runExpectingExit(t, func() {
		mockMCP(t, result, func() { runTransitionCommand([]string{"Decision", "D9", "--to", "superseded", "--rev", "1"}) })
	})
	if code != -1 || !strings.Contains(stderr, "2099-01-01") || !strings.Contains(stderr, "decision supersede") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

// TestDecisionSupersede_NewSources: the file's sources reach new.sources,
// inline on the sources line or from the JSON file it names; a bad sources
// line is refused before any request.
func TestDecisionSupersede_NewSources(t *testing.T) {
	dir := t.TempDir()
	src := `[{"repository":"acme/app","commit":"0123456789abcdef0123456789abcdef01234567","path":"a.md","role":"source"}]`
	if err := os.WriteFile(filepath.Join(dir, "sources.json"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, line := range map[string]string{"inline": src, "file": "sources.json"} {
		file := filepath.Join(dir, name+".md")
		if err := os.WriteFile(file, []byte("---\ntitle: Pinned\nsources: "+line+"\n---\nThe basis.\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, args, _ := mockMCP(t, map[string]any{"created": true}, func() {
			runDecisionCommand([]string{"supersede", "D186", "--new", "--from", file, "--reason", "r", "--rev", "3"})
		})
		n, _ := args["new"].(map[string]any)
		got, _ := n["sources"].([]any)
		first, _ := func() (map[string]any, bool) {
			if len(got) != 1 {
				return nil, false
			}
			m, ok := got[0].(map[string]any)
			return m, ok
		}()
		if first["repository"] != "acme/app" || first["path"] != "a.md" || first["role"] != "source" || n["title"] != "Pinned" {
			t.Errorf("%s: new = %v", name, n)
		}
	}
	for name, line := range map[string]string{"not JSON": "[a, b]", "missing file": "nope.json"} {
		file := filepath.Join(dir, "bad.md")
		if err := os.WriteFile(file, []byte("---\ntitle: Pinned\nsources: "+line+"\n---\nThe basis.\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, stderr := runExpectingExit(t, func() {
			runDecisionCommand([]string{"supersede", "D186", "--new", "--from", file, "--reason", "r", "--rev", "3"})
		})
		if code != 1 || !strings.Contains(stderr, "sources") {
			t.Errorf("%s: exit %d, stderr %q", name, code, stderr)
		}
	}
}
