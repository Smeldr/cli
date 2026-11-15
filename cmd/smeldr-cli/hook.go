package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The hook command adapts Claude Code's PreToolUse hook to decisions_for_paths
// (core T1207). Claude Code is the harness: it runs the command before an
// Edit, Write, MultiEdit or NotebookEdit call, with the call as JSON on stdin,
// and reads a permission decision from stdout. The command shows the agent the
// Decisions in force for the file before the first edit of it in a session.
// It does not check the change against them, and it does not see files
// written through the shell.

// hookInputLimit bounds what is read from stdin: a Write carries the whole
// file in tool_input.content.
const hookInputLimit = 8 << 20

// hookStateMaxAge is how long a session's shown-set is kept before the
// command removes it.
const hookStateMaxAge = 7 * 24 * time.Hour

// hookNow is the clock the state file's pruning reads; tests replace it.
var hookNow = time.Now

// preToolUseInput is the part of Claude Code's PreToolUse input the hook reads.
type preToolUseInput struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"tool_input"`
}

// preToolUseOutput is Claude Code's PreToolUse output: a permission decision,
// the reason shown to the agent on a deny, and context added on an allow.
type preToolUseOutput struct {
	HookSpecificOutput struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
		AdditionalContext        string `json:"additionalContext,omitempty"`
	} `json:"hookSpecificOutput"`
}

// hookAnswer is the decisions_for_paths answer the hook reads.
type hookAnswer struct {
	Areas []struct {
		Address      string `json:"address"`
		ID           string `json:"id"`
		Name         string `json:"name"`
		ApprovalMode string `json:"approval_mode"`
	} `json:"areas"`
	Decisions []struct {
		Address      string `json:"address"`
		ID           string `json:"id"`
		Title        string `json:"title"`
		Place        string `json:"place"`
		PlaceAddress string `json:"place_address"`
	} `json:"decisions"`
	Uncovered []string `json:"uncovered"`
	// Truncated and Next are not sent by decisions_for_paths today, which
	// answers every path it is given; if a server ever sends them, the hook
	// says the list is not complete.
	Truncated bool   `json:"truncated"`
	Next      string `json:"next"`
}

// hookOptions are the pre-edit command's flags.
type hookOptions struct {
	repo    string
	root    string
	onError string
	timeout time.Duration
}

// runHookCommand dispatches `smeldr-cli hook <kind>`.
func runHookCommand(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printHookHelp()
		return
	}
	switch args[0] {
	case "pre-edit":
		runPreEditHook(args[1:], stdin, os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "smeldr-cli hook: unknown hook %q\n\n", args[0])
		printHookHelp()
		os.Exit(1)
	}
}

// runPreEditHook reads one PreToolUse call from in and writes the decision to
// out. It always writes a decision and never exits non-zero: Claude Code lets
// the edit through on any exit code other than 0 and 2, so a failure here is
// turned into the --on-error decision instead.
func runPreEditHook(args []string, in io.Reader, out io.Writer) {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			printHookHelp()
			return
		}
	}
	opts := hookOptions{onError: "deny", timeout: 10 * time.Second}
	token := ""
	decide := func(o preToolUseOutput) error {
		o.HookSpecificOutput.HookEventName = "PreToolUse"
		b, _ := json.Marshal(o)
		_, err := fmt.Fprintln(out, string(b))
		return err
	}
	failed := func(err error) {
		decide(hookErrorOutput(opts.onError, scrubSecret(err.Error(), token)))
	}
	defer func() {
		if r := recover(); r != nil {
			failed(fmt.Errorf("the hook failed: %v", r))
		}
	}()

	fs := flag.NewFlagSet("hook pre-edit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.repo, "repo", "", "")
	fs.StringVar(&opts.root, "root", "", "")
	fs.StringVar(&opts.onError, "on-error", "deny", "")
	fs.DurationVar(&opts.timeout, "timeout", 10*time.Second, "")
	if err := fs.Parse(args); err != nil {
		opts.onError = "deny"
		failed(fmt.Errorf("smeldr-cli hook pre-edit: %v", err))
		return
	}
	if opts.onError != "deny" && opts.onError != "allow" {
		opts.onError = "deny"
		failed(fmt.Errorf("smeldr-cli hook pre-edit: --on-error must be deny or allow"))
		return
	}
	if opts.repo == "" {
		failed(fmt.Errorf("smeldr-cli hook pre-edit needs --repo owner/name in the hook's command"))
		return
	}

	raw, err := io.ReadAll(io.LimitReader(in, hookInputLimit+1))
	if err != nil || len(raw) > hookInputLimit {
		failed(fmt.Errorf("could not read the tool call from stdin"))
		return
	}
	raw = trimBOM(raw)
	var input preToolUseInput
	if err := json.Unmarshal(raw, &input); err != nil {
		failed(fmt.Errorf("the tool call on stdin is not Claude Code's PreToolUse JSON: %v", err))
		return
	}
	file := input.ToolInput.FilePath
	if file == "" {
		file = input.ToolInput.NotebookPath
	}
	if file == "" {
		failed(fmt.Errorf("the %s call names no file", input.ToolName))
		return
	}
	root := opts.root
	if root == "" {
		root = os.Getenv("CLAUDE_PROJECT_DIR")
	}
	if root == "" {
		root = input.Cwd
	}
	rel, inside := repoRelative(root, file, input.Cwd)
	if !inside {
		var o preToolUseOutput
		o.HookSpecificOutput.PermissionDecision = "allow"
		o.HookSpecificOutput.AdditionalContext = fmt.Sprintf("Smeldr: %s is outside the repository root %s, so it was not looked up in %s.", file, root, opts.repo)
		decide(o)
		return
	}

	// The project's own .smeldr-cli.env, when the hook runs from elsewhere.
	if root != "" {
		loadEnvFile(filepath.Join(root, ".smeldr-cli.env"))
	}
	cfg, err := loadConfig()
	if err != nil {
		failed(fmt.Errorf("smeldr-cli has no server to ask: %v (set SMELDR_URL and SMELDR_TOKEN, or run smeldr-cli init in the project)", err))
		return
	}
	token = cfg.Token
	// One deadline for the whole run: --timeout is the hook's budget, below
	// Claude Code's own timeout for the hook, so the hook decides what a slow
	// server means and never runs on. Each request gets what is left of it
	// (the requests run one after another, in this one process).
	deadline := time.Now().Add(opts.timeout)
	http.DefaultClient.Timeout = opts.timeout
	text, err := mcpCall(cfg, "decisions_for_paths", map[string]any{"repository": opts.repo, "paths": []string{rel}})
	if err != nil {
		failed(fmt.Errorf("the server at %s did not answer which Decisions apply to %s: %v", cfg.ForgeURL, rel, err))
		return
	}
	var ans hookAnswer
	if err := json.Unmarshal([]byte(text), &ans); err != nil {
		failed(fmt.Errorf("the server at %s gave an answer the hook cannot read: %v", cfg.ForgeURL, err))
		return
	}
	// "No Area" is allowed only when the answer says so itself, by listing the
	// path as uncovered. An answer that names no Area, no Decision and does
	// not list the path is one the hook cannot read: the error path.
	if len(ans.Areas) == 0 && len(ans.Decisions) == 0 && !contains(ans.Uncovered, rel) {
		failed(fmt.Errorf("the server at %s gave an answer the hook cannot read: it names no Area and no Decision for %s and does not list it as uncovered", cfg.ForgeURL, rel))
		return
	}
	// A Decision's text, read only for a deny, within what is left of the
	// deadline.
	body := func(ref string) (string, textResult) {
		left := time.Until(deadline)
		if left <= 0 {
			return "", textOutOfTime
		}
		http.DefaultClient.Timeout = left
		t, err := mcpCall(cfg, "get_decision", map[string]any{"slug": ref})
		if err != nil {
			var te interface{ Timeout() bool }
			if (errors.As(err, &te) && te.Timeout()) || time.Until(deadline) <= 0 {
				return "", textOutOfTime
			}
			return "", textUnreadable
		}
		var d struct {
			Body string `json:"body"`
		}
		if json.Unmarshal([]byte(t), &d) != nil || d.Body == "" {
			return "", textUnreadable
		}
		return d.Body, textRead
	}
	o, commit := hookDecision(ans, opts.repo, rel, input.SessionID, cfg.ForgeURL, body)
	// The path is marked as shown only once the deny is on stdout: if writing
	// it fails, the next edit is denied again.
	if decide(o) == nil && commit != nil {
		commit()
	}
}

// textResult says whether a Decision's text was read for the reason.
type textResult int

const (
	textRead textResult = iota
	textUnreadable
	textOutOfTime
)

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// hookBodyDecisions is how many Decisions the reason shows the text of, and
// hookBodyRunes how much of each: enough to read a rule, short enough for a
// tool result.
const (
	hookBodyDecisions = 10
	hookBodyRunes     = 600
)

// excerpt is s on one line, cut to n runes with an ellipsis.
func excerpt(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}

// hookDecision turns an answer into a decision. A path the answer lists as
// uncovered is allowed with that said; a covered path is denied once per
// session and set of Areas and Decisions, with them and their text in the
// reason, and allowed after. body reads a Decision's text and is called only
// for a deny. The returned commit, set only for a deny, records the path as
// shown; the caller runs it once the deny is written.
func hookDecision(ans hookAnswer, repo, rel, session, url string, body func(ref string) (string, textResult)) (preToolUseOutput, func()) {
	var o preToolUseOutput
	if len(ans.Areas) == 0 && len(ans.Decisions) == 0 {
		o.HookSpecificOutput.PermissionDecision = "allow"
		o.HookSpecificOutput.AdditionalContext = fmt.Sprintf("Smeldr named no Area for %s in %s to this token, so it showed no Decision for it. An Area this token may not read is left out.", rel, repo)
		return o, nil
	}
	key := shownKey(ans)
	state := readHookState(session)
	if state[rel] == key {
		o.HookSpecificOutput.PermissionDecision = "allow"
		return o, nil
	}
	commit := func() {
		state[rel] = key
		writeHookState(session, state)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Smeldr: before you edit %s, this is what is decided for it in %s (the Smeldr instance at %s).\n", rel, repo, url)
	for _, a := range ans.Areas {
		fmt.Fprintf(&b, "Area %s %s (approval: %s)\n", firstNonEmpty(a.Address, a.ID), a.Name, a.ApprovalMode)
	}
	if len(ans.Decisions) == 0 {
		b.WriteString("No Decision is in force in these places.\n")
	}
	outOfTime := false
	for i, d := range ans.Decisions {
		fmt.Fprintf(&b, "Decision %s (%s %s): %s\n", firstNonEmpty(d.Address, d.ID), d.Place, d.PlaceAddress, d.Title)
		if i >= hookBodyDecisions {
			continue
		}
		if outOfTime {
			b.WriteString("  (text not read: the hook's time ran out)\n")
			continue
		}
		switch text, res := body(firstNonEmpty(d.ID, d.Address)); res {
		case textRead:
			fmt.Fprintf(&b, "  Text: %s\n", excerpt(text, hookBodyRunes))
		case textUnreadable:
			b.WriteString("  (text not readable with this token: the title is not the whole rule)\n")
		case textOutOfTime:
			outOfTime = true
			b.WriteString("  (text not read: the hook's time ran out)\n")
		}
	}
	if len(ans.Decisions) > hookBodyDecisions {
		fmt.Fprintf(&b, "The text is shown for the first %d Decisions only.\n", hookBodyDecisions)
	}
	if outOfTime {
		b.WriteString("Not every text could be read within the hook's --timeout: read the rest in Smeldr before you rely on a title.\n")
	}
	if ans.Truncated || ans.Next != "" {
		fmt.Fprintf(&b, "The list is not complete: run smeldr-cli decisions-for-paths --repo %s %s for all of it.\n", repo, rel)
	}
	b.WriteString("These are the Decisions in force for this file. If your change keeps to them, make the same edit again now: this hook lets it through. If it would go against one, stop and ask the user instead. This hook does not check the change against them.")
	o.HookSpecificOutput.PermissionDecision = "deny"
	o.HookSpecificOutput.PermissionDecisionReason = b.String()
	return o, commit
}

// hookErrorOutput is the decision when the hook could not get an answer:
// deny with the reason (the default), or allow with it as context.
func hookErrorOutput(onError, msg string) preToolUseOutput {
	var o preToolUseOutput
	msg = strings.TrimRight(msg, ". ")
	if onError == "allow" {
		o.HookSpecificOutput.PermissionDecision = "allow"
		o.HookSpecificOutput.AdditionalContext = "Smeldr: " + msg + ". The edit was let through without the Decisions that apply (--on-error allow)."
		return o
	}
	o.HookSpecificOutput.PermissionDecision = "deny"
	o.HookSpecificOutput.PermissionDecisionReason = "Smeldr: " + msg + ". Edits are held until it answers. To edit anyway, the user can change --on-error to allow in the hook's command in .claude/settings.json."
	return o
}

// scrubSecret keeps the token out of a text that goes into the agent's
// transcript.
func scrubSecret(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "<token>")
}

// shownKey identifies what the agent was shown for a path: the addresses of
// the Areas and the Decisions, sorted.
func shownKey(ans hookAnswer) string {
	var parts []string
	for _, a := range ans.Areas {
		parts = append(parts, "A:"+firstNonEmpty(a.Address, a.ID))
	}
	for _, d := range ans.Decisions {
		parts = append(parts, "D:"+firstNonEmpty(d.Address, d.ID))
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

// repoRelative gives file relative to root, with forward slashes, and whether
// it is inside root. A relative file is taken from cwd. Windows paths (a drive
// letter, backslashes, or Git Bash's /c/ form) compare case-insensitively.
func repoRelative(root, file, cwd string) (string, bool) {
	f := slashPath(file)
	if !isAbsSlash(f) {
		base := cwd
		if base == "" {
			base = root
		}
		f = path.Join(slashPath(base), f)
	}
	f = path.Clean(f)
	if root == "" {
		return "", false
	}
	r := path.Clean(slashPath(root))
	fold := hasDrive(r) || hasDrive(f)
	if fold {
		if strings.EqualFold(f, r) {
			return "", false
		}
		if len(f) > len(r) && strings.EqualFold(f[:len(r)], r) && f[len(r)] == '/' {
			return f[len(r)+1:], true
		}
		if strings.HasSuffix(r, "/") && len(f) > len(r) && strings.EqualFold(f[:len(r)], r) {
			return f[len(r):], true
		}
		return "", false
	}
	if r == "/" {
		return strings.TrimPrefix(f, "/"), f != "/"
	}
	if strings.HasPrefix(f, r+"/") {
		return f[len(r)+1:], true
	}
	return "", false
}

// slashPath turns backslashes into slashes and Git Bash's /c/... into c:/...
func slashPath(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	if len(p) >= 3 && p[0] == '/' && p[2] == '/' && isLetter(p[1]) {
		p = string(p[1]) + ":" + p[2:]
	}
	if len(p) == 2 && hasDrive(p) {
		p += "/"
	}
	return p
}

func hasDrive(p string) bool { return len(p) >= 2 && p[1] == ':' && isLetter(p[0]) }

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func isAbsSlash(p string) bool { return strings.HasPrefix(p, "/") || hasDrive(p) }

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// hookStateDir is where each session's shown-set is kept.
// SMELDR_HOOK_STATE_DIR overrides it.
func hookStateDir() string {
	if d := os.Getenv("SMELDR_HOOK_STATE_DIR"); d != "" {
		return d
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "smeldr-cli", "hook")
	}
	return filepath.Join(os.TempDir(), "smeldr-cli-hook")
}

// hookStateFile names one session's file. A session ID is reduced to letters,
// digits, '-' and '_', so it cannot name a path elsewhere.
func hookStateFile(session string) string {
	var b strings.Builder
	for _, c := range session {
		if c == '-' || c == '_' || (c >= '0' && c <= '9') || (c < 128 && isLetter(byte(c))) {
			b.WriteRune(c)
		}
	}
	name := b.String()
	if name == "" {
		name = "no-session"
	}
	return filepath.Join(hookStateDir(), name+".json")
}

// readHookState reads a session's shown-set: path to shownKey. A missing or
// unreadable file is an empty set, so the next edit is denied again, never
// allowed by accident.
func readHookState(session string) map[string]string {
	state := map[string]string{}
	raw, err := os.ReadFile(hookStateFile(session))
	if err != nil {
		return state
	}
	if json.Unmarshal(raw, &state) != nil || state == nil {
		return map[string]string{}
	}
	return state
}

// writeHookState writes the shown-set through a temp file and a rename, so a
// parallel call reads the old file or the new one, never half of one. Two
// calls that race lose one entry at worst: the next edit of that path is
// denied once more. It also removes session files older than a week.
func writeHookState(session string, state map[string]string) {
	dir := hookStateDir()
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	b, err := json.Marshal(state)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".shown-*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), hookStateFile(session)) != nil {
		os.Remove(tmp.Name()) //nolint:errcheck
	}
	pruneHookState(dir)
}

// pruneHookState removes session files not written for hookStateMaxAge.
func pruneHookState(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() {
			continue
		}
		if hookNow().Sub(info.ModTime()) > hookStateMaxAge {
			os.Remove(filepath.Join(dir, e.Name())) //nolint:errcheck
		}
	}
}

func printHookHelp() {
	fmt.Print(`smeldr-cli hook pre-edit: show an agent the Decisions for a file before it edits it

Usage:
  smeldr-cli hook pre-edit --repo owner/name [--root <dir>] [--on-error deny|allow] [--timeout 10s]

A Claude Code PreToolUse hook: it reads Claude Code's PreToolUse JSON on stdin
and writes Claude Code's PreToolUse decision on stdout. It is not a general
agent interface. In .claude/settings.json:

  {"hooks": {"PreToolUse": [{"matcher": "Edit|Write|MultiEdit|NotebookEdit",
    "hooks": [{"type": "command", "command": "smeldr-cli hook pre-edit --repo acme/app", "timeout": 30}]}]}}

For each edit it asks decisions_for_paths about the file, relative to --root
(default $CLAUDE_PROJECT_DIR, else the session's directory):
  - a file an Area covers: the first edit in a session is denied, with the
    Areas and the Decisions in force, and their text, in the reason; the same
    edit again is allowed. A new Area or Decision on the file denies once more.
  - a file no Area names to this token: allowed, with that said to the agent.
    An Area the token may not read is left out.
  - no answer (no config, server down, a refusal, an answer of a shape the
    hook does not know): denied, with the reason (--on-error deny, the
    default), or allowed with a warning (--on-error allow).
--timeout is the budget of the whole run (keep it below the timeout in
settings.json): texts not read in time are marked so in the reason.

Claude Code enforces the deny, not the agent. The hook does not check the
change against the Decisions, and it does not see files written through the
shell. Config is SMELDR_URL and SMELDR_TOKEN, or .smeldr-cli.env in the
project; the token needs the Author role and read on the places that matter.
What was shown is kept per session under the user cache directory
(SMELDR_HOOK_STATE_DIR overrides it) and removed after 7 days.
`)
}
