package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// grantUsage is the help for the grant commands.
const grantUsage = `smeldr-cli grant: governance role grants (Admin role required)

Usage:
  grant <token-id> <role> [--scope type:id ...] [--anchor <id>] [--expires-in-days N] [--reason <text>]
                                     grant a role to a token
  grant revoke <grant-id> [--reason <text>]
                                     revoke a grant by its own id
  grant list [<token-id>]            list all grants, or one token's

<token-id> is the token's JWT user id (token_id from "token create"), not the
fingerprint "token list" shows. --scope may repeat (static scope patterns,
"type:id" or "type:*"); --anchor is the anchor item id for a dynamic scope.
--expires-in-days makes the grant time-boxed (a positive number of days): when
it expires it stops authorizing, with no revoke needed, and "grant list" still
shows it.
--reason is stored with the act and shown by "grant list". It is free text that
people read: never put a token value or other secret in it.

The MCP endpoint is used (SMELDR_MCP_URL).
`

// runGrantCommand dispatches the grant commands.
func runGrantCommand(args []string) {
	verb, params, err := parseGrantArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(os.Stdout, grantUsage)
		return
	}
	if err != nil {
		fatal("%v", err)
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, verb, params)
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// maxExpiryDays is the server's cap on a days argument (100 years), checked
// here too so an out-of-range value fails before any request.
const maxExpiryDays = 36500

// stringsFlag collects a repeated string flag.
type stringsFlag []string

func (s *stringsFlag) String() string     { return strings.Join(*s, ",") }
func (s *stringsFlag) Set(v string) error { *s = append(*s, v); return nil }

// parseGrantArgs turns the grant command line into the MCP tool to call and its
// arguments. help, -h and --help return flag.ErrHelp; a missing argument or an
// unknown flag is an error. Nothing is sent before it returns.
func parseGrantArgs(args []string) (tool string, params map[string]any, err error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("grant requires: <token-id> <role>, or revoke <grant-id>, or list [<token-id>]")
	}
	switch args[0] {
	case "help", "-h", "--help":
		return "", nil, flag.ErrHelp
	case "revoke":
		pos, reason, _, _, _, err := parseGrantFlags("grant revoke", args[1:], 1, false)
		if err != nil {
			return "", nil, err
		}
		if len(pos) != 1 {
			return "", nil, fmt.Errorf("grant revoke requires a grant id")
		}
		p := map[string]any{"id": pos[0]}
		if reason != "" {
			p["reason"] = reason
		}
		return "revoke_grant", p, nil
	case "list":
		if len(args) > 2 {
			return "", nil, fmt.Errorf("grant list takes at most one token id")
		}
		p := map[string]any{}
		if len(args) == 2 {
			if strings.HasPrefix(args[1], "-") {
				if args[1] == "-h" || args[1] == "--help" {
					return "", nil, flag.ErrHelp
				}
				return "", nil, fmt.Errorf("unknown flag %q", args[1])
			}
			p["token_id"] = args[1]
		}
		return "list_grants", p, nil
	}
	pos, reason, scopes, anchor, days, err := parseGrantFlags("grant", args, 2, true)
	if err != nil {
		return "", nil, err
	}
	if len(pos) != 2 {
		return "", nil, fmt.Errorf("grant requires: <token-id> <role>")
	}
	p := map[string]any{"token_id": pos[0], "role": pos[1]}
	if len(scopes) > 0 {
		list := make([]any, len(scopes))
		for i, s := range scopes {
			list[i] = s
		}
		p["scope_static"] = list
	}
	if anchor != "" {
		p["scope_anchor_id"] = anchor
	}
	if days != "" {
		n, convErr := strconv.ParseFloat(days, 64)
		if convErr != nil || n <= 0 || n > maxExpiryDays {
			return "", nil, fmt.Errorf("--expires-in-days must be a positive number of days, at most %d, got %q", maxExpiryDays, days)
		}
		p["expires_in_days"] = n
	}
	if reason != "" {
		p["reason"] = reason
	}
	return "grant_role", p, nil
}

// parseGrantFlags reads flags before or after up to want positional arguments.
// withScope adds --scope and --anchor.
func parseGrantFlags(name string, args []string, want int, withScope bool) (pos []string, reason string, scopes stringsFlag, anchor, days string, err error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, grantUsage) }
	fs.StringVar(&reason, "reason", "", "why (never a secret)")
	if withScope {
		fs.Var(&scopes, "scope", `static scope pattern "type:id" or "type:*" (repeatable)`)
		fs.StringVar(&anchor, "anchor", "", "anchor item id for a dynamic scope")
		fs.StringVar(&days, "expires-in-days", "", "time-box the grant: expires after this many days")
	}
	if err = fs.Parse(args); err != nil {
		return nil, "", nil, "", "", err
	}
	pos = fs.Args()
	if len(pos) > want {
		// Flags after the positional arguments: parse the rest again.
		if err = fs.Parse(pos[want:]); err != nil {
			return nil, "", nil, "", "", err
		}
		if fs.NArg() > 0 {
			return nil, "", nil, "", "", fmt.Errorf("unexpected argument %q", fs.Arg(0))
		}
		pos = pos[:want]
	}
	return pos, reason, scopes, anchor, days, nil
}
