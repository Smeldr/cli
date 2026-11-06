package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// helpChildEnv makes the test binary run main() instead of the tests, so a
// test can run smeldr-cli as a process with an empty environment.
const helpChildEnv = "SMELDR_CLI_HELP_CHILD"

func TestMain(m *testing.M) {
	if os.Getenv(helpChildEnv) == "1" {
		os.Args = append([]string{"smeldr-cli"}, strings.Fields(os.Getenv(helpChildEnv+"_ARGS"))...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runCLI runs smeldr-cli args as a child process with no SMELDR_* variables
// and an empty home, and returns its stdout, stderr and exit code.
func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	home := t.TempDir()
	cmd := exec.Command(os.Args[0])
	cmd.Dir = home
	cmd.Env = []string{
		helpChildEnv + "=1", helpChildEnv + "_ARGS=" + strings.Join(args, " "),
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home,
		"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"),
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), errOut.String(), code
}

// topLevelCommands reads the cases of main.go's command switch, so a new
// command is covered without a second list to keep.
func topLevelCommands(t *testing.T) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(".", "main.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var cmds []string
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "main" {
			return true
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				lit, ok := e.(*ast.BasicLit)
				if !ok {
					continue
				}
				name, _ := strconv.Unquote(lit.Value)
				if !strings.HasPrefix(name, "-") && name != "help" && name != "version" {
					cmds = append(cmds, name)
				}
			}
			return true
		})
		return false
	})
	if len(cmds) < 30 {
		t.Fatalf("found only %d commands in main.go's switch: %v", len(cmds), cmds)
	}
	return cmds
}

// TestHelp_EveryTopLevelCommand: every command in main.go's switch, and a
// content path, answers --help with its own help on stdout, exit 0, nothing
// on stderr, and no server or token (T1147).
func TestHelp_EveryTopLevelCommand(t *testing.T) {
	for _, cmd := range append(topLevelCommands(t), "posts") {
		t.Run(cmd, func(t *testing.T) {
			out, errOut, code := runCLI(t, cmd, "--help")
			if code != 0 || errOut != "" {
				t.Fatalf("exit %d, stderr %q", code, errOut)
			}
			first, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
			if !strings.Contains(first, "smeldr-cli "+cmd) && !strings.Contains(first, "smeldr-cli init") {
				t.Errorf("first line %q does not name smeldr-cli %s", first, cmd)
			}
		})
	}
}

// TestHelp_DecisionListsSources: decision --help is the decision command's
// own help, under its own name, with sources.
func TestHelp_DecisionListsSources(t *testing.T) {
	out, _, code := runCLI(t, "decision", "--help")
	if code != 0 || !strings.HasPrefix(out, "smeldr-cli decision:") || !strings.Contains(out, "decision sources") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}

// TestHelp_BadFlagStillStderr: a wrong flag is an error on stderr with exit
// 1; only help that was asked for goes to stdout.
func TestHelp_BadFlagStillStderr(t *testing.T) {
	out, errOut, code := runCLI(t, "status", "--nope")
	if code != 1 || !strings.Contains(errOut, "nope") || out != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

// TestHelp_EveryCommandInUsage: every command in main.go's switch has a line
// in the overview's Usage block, which docs' generator reads (T1147).
func TestHelp_EveryCommandInUsage(t *testing.T) {
	out, _, code := runCLI(t, "--help")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	usage, _, _ := strings.Cut(out, "\n\n")
	if _, rest, ok := strings.Cut(out, "Usage:\n"); ok {
		usage, _, _ = strings.Cut(rest, "\n\n")
	}
	for _, cmd := range topLevelCommands(t) {
		if !strings.Contains(usage, "  smeldr-cli "+cmd+" ") && !strings.Contains(usage, "  smeldr-cli "+cmd+"\n") && !strings.HasSuffix(usage, "  smeldr-cli "+cmd) {
			t.Errorf("no Usage line for %s", cmd)
		}
	}
}
