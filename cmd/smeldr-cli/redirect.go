package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// runRedirectCommand dispatches redirect subcommands. args begins with the verb.
func runRedirectCommand(args []string) {
	if len(args) == 0 {
		printRedirectHelp()
		os.Exit(1)
	}
	switch args[0] {
	case "-h", "--help", "help":
		printRedirectHelp()
	case "list":
		runRedirectList(args[1:])
	case "create":
		runRedirectCreate(args[1:])
	case "delete":
		runRedirectDelete(args[1:])
	default:
		fatal("unknown redirect verb %q — use: list create delete", args[0])
	}
}

func printRedirectHelp() {
	fmt.Fprint(os.Stdout, `smeldr-cli redirect — runtime redirect management (Editor role required)

Verbs:
  list                                         list all redirect rules
  list --json                                  list redirects as JSON
  create --from <path> --to <path>             create a 301 redirect
  create --from <path> --to <path> --code 302  create a 302 redirect
  create --from <path> --code 410              create a 410 Gone rule
  create --from <path> --to <path> --prefix    create a prefix redirect
  delete <from-path>                           delete a redirect rule

The MCP endpoint is used for redirect operations (SMELDR_MCP_URL).
`)
}

// runRedirectList lists all redirect rules via list_redirects MCP tool.
func runRedirectList(args []string) {
	fs := newCommandFlags("redirect list")
	asJSON := fs.Bool("json", false, "raw JSON")
	parseCommandFlags(fs, args, 0, "")
	jsonFlag := *asJSON

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}

	text, err := mcpCall(cfg, "list_redirects", map[string]any{})
	if err != nil {
		fatal("%v", err)
	}
	if jsonFlag {
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
		return
	}
	table, err := redirectListTable(text)
	if err != nil {
		fatal("%v", err)
	}
	fmt.Print(table)
}

// runRedirectCreate creates a redirect rule via create_redirect MCP tool.
func runRedirectCreate(args []string) {
	fs := newCommandFlags("redirect create")
	fromFlag := fs.String("from", "", "the path to redirect")
	toFlag := fs.String("to", "", "the target")
	codeFlag := fs.Int("code", 0, "the HTTP status code")
	prefix := fs.Bool("prefix", false, "match the path as a prefix")
	parseCommandFlags(fs, args, 0, "")
	from, to, code, isPrefix := *fromFlag, *toFlag, *codeFlag, *prefix
	if from == "" {
		fatal("redirect create requires --from <path>")
	}

	callArgs := map[string]any{
		"from":      from,
		"to":        to,
		"is_prefix": isPrefix,
	}
	if code != 0 {
		callArgs["code"] = float64(code)
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "create_redirect", callArgs)
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// runRedirectDelete deletes a redirect rule via delete_redirect MCP tool.
func runRedirectDelete(args []string) {
	from := oneArgument("redirect delete", args, "redirect delete requires a from-path argument")

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "delete_redirect", map[string]any{"from": from})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// redirectListTable renders a list_redirects JSON result as an aligned table
// with columns FROM, TO, CODE, PREFIX. An empty list yields a short notice.
func redirectListTable(jsonText string) (string, error) {
	var resp struct {
		Redirects []map[string]any `json:"redirects"`
	}
	if err := json.Unmarshal([]byte(jsonText), &resp); err != nil {
		return "", fmt.Errorf("decode list_redirects response: %w", err)
	}
	if len(resp.Redirects) == 0 {
		return "No redirects.\n", nil
	}
	rows := make([][]string, 0, len(resp.Redirects))
	for _, r := range resp.Redirects {
		from := asString(r["from"])
		to := asString(r["to"])
		code := fmt.Sprintf("%v", r["code"])
		prefix := "no"
		if b, ok := r["is_prefix"].(bool); ok && b {
			prefix = "yes"
		}
		rows = append(rows, []string{from, to, code, prefix})
	}
	return renderTable([]string{"FROM", "TO", "CODE", "PREFIX"}, rows), nil
}
