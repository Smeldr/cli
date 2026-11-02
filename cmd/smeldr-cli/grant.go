package main

import (
	"encoding/json"
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
  grant <token-id> <role> [--scope type:id ...] [--anchor <id>] [--expires-in-days N] [--reason <text>] [--rev <n>]
                                     grant a role to a token
  grant revoke <grant-id> [--reason <text>] [--rev <n>]
                                     revoke a grant by its own id
  grant list [<token-id>]            list all grants, or one token's

<token-id> is the token's JWT user id (token_id from "token create"), not the
fingerprint "token list" shows. --scope may repeat (static scope patterns,
"type:id" or "type:*"); --anchor is the anchor item id for a dynamic scope.
--expires-in-days makes the grant time-boxed (a positive number of days): when
it expires it stops authorizing, with no revoke needed, and "grant list" still
shows it.
--rev is the rev of the holder's identity you read (identity_rev in "grant list
<token-id>"): a grant or revoke on a stale view is refused. Without --rev, on a
terminal, cli shows the identity's grants and asks; without a terminal it
refuses. A holder that is not an identity yet names no rev.
--reason is stored with the act and shown by "grant list". It is free text that
people read: never put a token value or other secret in it.

The MCP endpoint is used (SMELDR_MCP_URL).
`

// runGrantCommand dispatches the grant commands.
func runGrantCommand(args []string) {
	args, rev, err := takeRev(args)
	if err != nil {
		fatal("%v", err)
		return
	}
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
	if verb == "list_grants" {
		if rev != nil {
			fatal("--rev is for grant and grant revoke, not grant list")
			return
		}
		text, err := mcpCall(cfg, verb, params)
		if err != nil {
			fatal("%v", err)
		}
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
		return
	}
	// A grant or revoke names the rev of the holder's identity (core T1010).
	if rev == nil {
		rev = confirmGrantRev(cfg, verb, params)
	}
	if rev != nil {
		params["rev"] = *rev
	}
	text := writeCall(cfg, verb, params, nil)
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// confirmGrantRev reads the holder's grants and its identity's rev, shows
// them and asks; nil when the holder is not an identity (no rev to name).
func confirmGrantRev(cfg Config, verb string, params map[string]any) *int {
	holder, _ := params["token_id"].(string)
	if verb == "revoke_grant" {
		holder = grantHolder(cfg, params["id"].(string))
		if holder == "" {
			return nil // an unknown grant: the server answers
		}
	}
	text, err := mcpCall(cfg, "list_grants", map[string]any{"token_id": holder})
	if err != nil {
		fatal("%v", err)
		return nil
	}
	var res struct {
		Grants []struct {
			ID       string `json:"ID"`
			RoleName string `json:"RoleName"`
		} `json:"grants"`
		IdentityRev *int `json:"identity_rev"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		fatal("unexpected list_grants response: %v", err)
		return nil
	}
	if res.IdentityRev == nil {
		return nil // not an identity yet: no rev to name
	}
	needTerminal("grant", "identity_rev from grant list "+holder)
	roles := make([]string, 0, len(res.Grants))
	for _, g := range res.Grants {
		roles = append(roles, g.RoleName)
	}
	action := "Grant " + fmt.Sprint(params["role"]) + " to"
	if verb == "revoke_grant" {
		action = "Revoke grant " + fmt.Sprint(params["id"]) + " from"
	}
	confirmRev(action, fmt.Sprintf("identity %s, grants now: %s", holder, strings.Join(roles, ", ")), *res.IdentityRev)
	return res.IdentityRev
}

// grantHolder finds the token_id holding grantID, "" when none.
func grantHolder(cfg Config, grantID string) string {
	text, err := mcpCall(cfg, "list_grants", map[string]any{})
	if err != nil {
		fatal("%v", err)
		return ""
	}
	var res struct {
		Grants []struct {
			ID      string `json:"ID"`
			TokenID string `json:"TokenID"`
		} `json:"grants"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		fatal("unexpected list_grants response: %v", err)
		return ""
	}
	for _, g := range res.Grants {
		if g.ID == grantID {
			return g.TokenID
		}
	}
	return ""
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
	if pos, err = parseAtMost(fs, args, want); err != nil {
		return nil, "", nil, "", "", err
	}
	return pos, reason, scopes, anchor, days, nil
}
