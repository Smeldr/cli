package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
)

// parseSignalGetArgs reads `signal get <address> [--json]`.
func parseSignalGetArgs(args []string) (string, bool, error) {
	c, err := splitArgs(args, map[string]bool{"json": true})
	if err != nil {
		return "", false, err
	}
	if err := c.only("signal get", "json"); err != nil {
		return "", false, err
	}
	addr, err := c.one("signal get", "address")
	if err != nil {
		return "", false, err
	}
	asJSON, err := jsonFlag(c)
	return addr, asJSON, err
}

// signalListArgs is a parsed `signal list`: list_signals' arguments and --json.
type signalListArgs struct {
	params map[string]any
	toMe   bool
	json   bool
}

// parseSignalListArgs reads `signal list [--receiver r | --sender s] [--state
// s] [--limit n] [--offset n] [--json]`. Without --receiver and --sender it
// asks for what is addressed to the caller.
func parseSignalListArgs(args []string) (signalListArgs, error) {
	c, err := splitArgs(args, map[string]bool{"json": true})
	if err != nil {
		return signalListArgs{}, err
	}
	if err := c.only("signal list", "receiver", "sender", "state", "limit", "offset", "json"); err != nil {
		return signalListArgs{}, err
	}
	if len(c.pos) > 0 {
		return signalListArgs{}, fmt.Errorf("signal list: unexpected argument %q", c.pos[0])
	}
	params := map[string]any{}
	for _, k := range []string{"receiver", "sender", "state"} {
		if v, ok := c.flags[k]; ok {
			if v == "" {
				return signalListArgs{}, fmt.Errorf("--%s needs a value", k)
			}
			params[k] = v
		}
	}
	for _, k := range []string{"limit", "offset"} {
		v, ok := c.flags[k]
		if !ok {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return signalListArgs{}, fmt.Errorf("--%s must be a whole number, got %q", k, v)
		}
		params[k] = n
	}
	out := signalListArgs{params: params}
	if params["receiver"] == nil && params["sender"] == nil {
		params["addressed_to_me"] = true
		out.toMe = true
	}
	out.json, err = jsonFlag(c)
	return out, err
}

// runSignalGet prints one Signal.
func runSignalGet(args []string) {
	addr, asJSON, err := parseSignalGetArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Print(signalUsage)
		return
	}
	if err != nil {
		fatal("%v", err)
		return
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "get_signal", map[string]any{"slug": addr})
	if err != nil {
		fatal("%v", err)
		return
	}
	if asJSON {
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
		return
	}
	var s map[string]any
	if err := json.Unmarshal([]byte(text), &s); err != nil {
		fatal("unexpected response: %v", err)
		return
	}
	printSignal(os.Stdout, s)
}

// printSignal writes one Signal as labelled lines: the empty ones are left out.
func printSignal(w io.Writer, s map[string]any) {
	str := func(k string) string { return strOf(s, k) }
	line := func(label, v string) {
		if v != "" {
			fmt.Fprintf(w, "%-10s %s\n", label+":", v)
		}
	}
	line("Signal", str("address"))
	line("Type", str("signal_type"))
	line("State", str("Status"))
	line("From", str("sender"))
	line("To", str("receiver"))
	if t := str("subject_type"); t != "" {
		line("Subject", strings.TrimSpace(t+" "+firstNonEmpty(str("subject_address"), str("subject_id"))))
	}
	line("Task", str("task_ref"))
	line("Message", str("message"))
	line("Answer", str("answer"))
	line("Ref", str("answer_ref"))
	line("Resolution", str("resolution"))
}

// strOf is m[k] when it is a string, else "".
func strOf(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

// signalListPage is one list_signals result.
type signalListPage struct {
	Signals []map[string]any `json:"signals"`
	Count   int              `json:"count"`
	Total   int              `json:"total"`
}

// runSignalList lists Signals: what is addressed to me by default.
func runSignalList(args []string) {
	la, err := parseSignalListArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Print(signalUsage)
		return
	}
	if err != nil {
		fatal("%v", err)
		return
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "list_signals", la.params)
	if err != nil {
		if la.toMe && olderListSignals(err) {
			fatal("this server's list_signals cannot list what is addressed to you yet (it needs smeldr.dev/mcp with addressed_to_me): name a receiver, such as signal list --receiver <your band>")
			return
		}
		fatal("%v", err)
		return
	}
	if la.json {
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
		return
	}
	var p signalListPage
	if err := json.Unmarshal([]byte(text), &p); err != nil {
		fatal("unexpected response: %v", err)
		return
	}
	printSignalTable(os.Stdout, p)
}

// olderListSignals reports whether err is an mcp from before addressed_to_me
// refusing it.
func olderListSignals(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "addressed_to_me") || strings.Contains(msg, "at least one of receiver or sender")
}

// printSignalTable writes the Signals as a table, the message cut to 60
// characters.
func printSignalTable(w io.Writer, p signalListPage) {
	if len(p.Signals) == 0 {
		fmt.Fprintln(w, "No Signals.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SIGNAL\tSTATE\tTYPE\tTO\tSUBJECT\tMESSAGE")
	for _, s := range p.Signals {
		subject := ""
		if t := strOf(s, "subject_type"); t != "" {
			subject = strings.TrimSpace(t + " " + firstNonEmpty(strOf(s, "subject_address"), strOf(s, "subject_id")))
		}
		msg := strOf(s, "message")
		if r := []rune(msg); len(r) > 60 {
			msg = string(r[:57]) + "..."
		}
		// An mcp from before addresses on list_signals rows sends no
		// address: the slug, then the ID, name the Signal.
		name := firstNonEmpty(strOf(s, "address"), firstNonEmpty(strOf(s, "slug"), strOf(s, "id")))
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", name, strOf(s, "status"), strOf(s, "signal_type"), strOf(s, "receiver"), subject, strings.ReplaceAll(msg, "\n", " "))
	}
	tw.Flush()
	fmt.Fprintf(w, "%d of %d Signals\n", len(p.Signals), p.Total)
}
