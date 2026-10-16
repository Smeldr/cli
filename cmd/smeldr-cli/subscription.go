package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// subscriptionUsage is the help for the subscription commands.
const subscriptionUsage = `smeldr-cli subscription: your event subscriptions (D116, Author role)

Usage:
  subscription list [--owner <user-id>]          your subscriptions, the three defaults included
  subscription get <address>                     one subscription (N12, slug or ID)
  subscription create --name <label> [--filter <json>|@<file>] [--include-own] [--paused] [--owner <user-id>]
  subscription update <address> [--name <label>] [--filter <json>|@<file>] [--include-own=true|false]
  subscription pause <address>
  subscription resume <address>
  subscription delete <address>                  the defaults cannot be deleted; pause them

A subscription is what "smeldr-cli watch --subscription <address>" receives.
--filter is a JSON object: types, events, event_names, from_states, to_states,
items, related_to, fields, signal_kinds, addressed_to_me, sent_by_me; an
unknown key is refused. --owner names another token, which needs administer.

The MCP endpoint is used (SMELDR_MCP_URL). Needs smeldr.dev/mcp v1.64.0 or later.
`

// signalUsage is the help for the signal commands.
const signalUsage = `smeldr-cli signal: answer or withdraw an open request (D115, Author role)

Usage:
  signal answer <address> --text <answer> [--ref <Type:ident>]
  signal withdraw <address> --reason <text>

--ref names the change the answer made, such as Task:T42.
The MCP endpoint is used (SMELDR_MCP_URL). Needs smeldr.dev/mcp v1.64.0 or later.
`

// runSubscriptionCommand dispatches the subscription commands.
func runSubscriptionCommand(args []string) {
	runToolCommand(args, subscriptionUsage, parseSubscriptionArgs)
}

// runSignalCommand dispatches the signal commands.
func runSignalCommand(args []string) {
	runToolCommand(args, signalUsage, parseSignalArgs)
}

// runToolCommand parses args into one MCP tool call, sends it and prints the
// result as JSON. flag.ErrHelp prints usage.
func runToolCommand(args []string, usage string, parse func([]string) (string, map[string]any, error)) {
	tool, params, err := parse(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(os.Stdout, usage)
		return
	}
	if err != nil {
		fatal("%v", err)
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, tool, params)
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// cmdArgs is a command line split into positional arguments and --flags.
type cmdArgs struct {
	pos   []string
	flags map[string]string
}

// splitArgs separates positionals from --name value and --name=value flags.
// A flag in boolean takes no value unless written --name=value.
func splitArgs(args []string, boolean map[string]bool) (cmdArgs, error) {
	out := cmdArgs{flags: map[string]string{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-h" || a == "--help" || a == "help" {
			return out, flag.ErrHelp
		}
		if !strings.HasPrefix(a, "--") {
			out.pos = append(out.pos, a)
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		if !hasValue {
			if boolean[name] {
				value = "true"
			} else {
				if i+1 >= len(args) {
					return out, fmt.Errorf("--%s needs a value", name)
				}
				i++
				value = args[i]
			}
		}
		out.flags[name] = value
	}
	return out, nil
}

// only reports an error naming the first flag not in allowed.
func (c cmdArgs) only(cmd string, allowed ...string) error {
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	for f := range c.flags {
		if !ok[f] {
			return fmt.Errorf("%s: unknown flag --%s", cmd, f)
		}
	}
	return nil
}

// one returns the single positional argument cmd needs.
func (c cmdArgs) one(cmd, what string) (string, error) {
	if len(c.pos) != 1 {
		return "", fmt.Errorf("%s requires <%s>", cmd, what)
	}
	return c.pos[0], nil
}

// boolFlag reads a true/false flag value.
func boolFlag(name, v string) (bool, error) {
	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("--%s takes true or false, not %q", name, v)
}

// readFilter reads --filter: a JSON object, or @file for one in a file.
func readFilter(v string) (map[string]any, error) {
	raw := []byte(v)
	if path, ok := strings.CutPrefix(v, "@"); ok {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("--filter: %w", err)
		}
		raw = b
	}
	var f map[string]any
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("--filter must be a JSON object: %w", err)
	}
	return f, nil
}

// parseSubscriptionArgs turns the subscription command line into the MCP tool
// to call and its arguments. Nothing is sent before it returns.
func parseSubscriptionArgs(args []string) (string, map[string]any, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("subscription requires a verb: list, get, create, update, pause, resume or delete")
	}
	verb := args[0]
	if verb == "help" || verb == "-h" || verb == "--help" {
		return "", nil, flag.ErrHelp
	}
	c, err := splitArgs(args[1:], map[string]bool{"include-own": true, "paused": true})
	if err != nil {
		return "", nil, err
	}
	cmd := "subscription " + verb
	params := map[string]any{}
	switch verb {
	case "list":
		if err := c.only(cmd, "owner"); err != nil {
			return "", nil, err
		}
		if len(c.pos) != 0 {
			return "", nil, fmt.Errorf("%s takes no arguments", cmd)
		}
		if v, ok := c.flags["owner"]; ok {
			params["owner"] = v
		}
		return "list_subscriptions", params, nil
	case "get", "delete", "pause", "resume":
		if err := c.only(cmd); err != nil {
			return "", nil, err
		}
		addr, err := c.one(cmd, "address")
		if err != nil {
			return "", nil, err
		}
		params["subscription"] = addr
		switch verb {
		case "get":
			return "get_subscription", params, nil
		case "delete":
			return "delete_subscription", params, nil
		case "pause":
			params["paused"] = true
		default:
			params["paused"] = false
		}
		return "update_subscription", params, nil
	case "create":
		if err := c.only(cmd, "name", "filter", "include-own", "paused", "owner"); err != nil {
			return "", nil, err
		}
		if len(c.pos) != 0 {
			return "", nil, fmt.Errorf("%s takes flags only", cmd)
		}
		name := c.flags["name"]
		if name == "" {
			return "", nil, fmt.Errorf("%s requires --name", cmd)
		}
		params["name"] = name
		if v, ok := c.flags["owner"]; ok {
			params["owner"] = v
		}
		for _, b := range []string{"include-own", "paused"} {
			if v, ok := c.flags[b]; ok {
				on, err := boolFlag(b, v)
				if err != nil {
					return "", nil, err
				}
				params[strings.ReplaceAll(b, "-", "_")] = on
			}
		}
		if v, ok := c.flags["filter"]; ok {
			f, err := readFilter(v)
			if err != nil {
				return "", nil, err
			}
			params["filter"] = f
		}
		return "create_subscription", params, nil
	case "update":
		if err := c.only(cmd, "name", "filter", "include-own"); err != nil {
			return "", nil, err
		}
		addr, err := c.one(cmd, "address")
		if err != nil {
			return "", nil, err
		}
		params["subscription"] = addr
		if v, ok := c.flags["name"]; ok {
			params["name"] = v
		}
		if v, ok := c.flags["include-own"]; ok {
			on, err := boolFlag("include-own", v)
			if err != nil {
				return "", nil, err
			}
			params["include_own"] = on
		}
		if v, ok := c.flags["filter"]; ok {
			f, err := readFilter(v)
			if err != nil {
				return "", nil, err
			}
			params["filter"] = f
		}
		if len(params) == 1 {
			return "", nil, fmt.Errorf("%s needs --name, --filter or --include-own (pause and resume have their own verbs)", cmd)
		}
		return "update_subscription", params, nil
	}
	return "", nil, fmt.Errorf("unknown subscription verb %q: list, get, create, update, pause, resume or delete", verb)
}

// parseSignalArgs turns the signal command line into answer_signal or
// withdraw_signal and its arguments.
func parseSignalArgs(args []string) (string, map[string]any, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("signal requires a verb: answer or withdraw")
	}
	verb := args[0]
	if verb == "help" || verb == "-h" || verb == "--help" {
		return "", nil, flag.ErrHelp
	}
	c, err := splitArgs(args[1:], nil)
	if err != nil {
		return "", nil, err
	}
	cmd := "signal " + verb
	switch verb {
	case "answer":
		if err := c.only(cmd, "text", "ref"); err != nil {
			return "", nil, err
		}
		addr, err := c.one(cmd, "address")
		if err != nil {
			return "", nil, err
		}
		if c.flags["text"] == "" {
			return "", nil, fmt.Errorf("%s requires --text", cmd)
		}
		params := map[string]any{"signal": addr, "answer": c.flags["text"]}
		if v, ok := c.flags["ref"]; ok {
			params["answer_ref"] = v
		}
		return "answer_signal", params, nil
	case "withdraw":
		if err := c.only(cmd, "reason"); err != nil {
			return "", nil, err
		}
		addr, err := c.one(cmd, "address")
		if err != nil {
			return "", nil, err
		}
		if c.flags["reason"] == "" {
			return "", nil, fmt.Errorf("%s requires --reason", cmd)
		}
		return "withdraw_signal", map[string]any{"signal": addr, "reason": c.flags["reason"]}, nil
	}
	return "", nil, fmt.Errorf("unknown signal verb %q: answer or withdraw", verb)
}
