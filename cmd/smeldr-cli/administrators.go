package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
)

// runAdministratorsCommand lists who administers the instance via the
// administrators MCP tool (core T944, D154).
func runAdministratorsCommand(args []string) {
	raw := false
	for _, a := range args {
		switch a {
		case "-h", "--help", "help":
			printAdministratorsHelp()
			return
		case "--json":
			raw = true
		default:
			fatal("administrators takes no arguments, got %q", a)
		}
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "administrators", map[string]any{})
	if err != nil {
		fatal("%v", err)
	}
	if raw {
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
		return
	}
	var res struct {
		Administrators []struct {
			Name  string `json:"name"`
			Class string `json:"class"`
		} `json:"administrators"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		fatal("unexpected response: %v", err)
	}
	if len(res.Administrators) == 0 {
		fmt.Println("nobody administers this instance")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCLASS")
	for _, a := range res.Administrators {
		fmt.Fprintf(w, "%s\t%s\n", a.Name, a.Class)
	}
	w.Flush()
}

func printAdministratorsHelp() {
	fmt.Print(`smeldr-cli administrators: who administers this instance

Usage:
  smeldr-cli administrators [--json]

Lists who administers the instance: token name and class (human, agent, job,
unclassified), nothing else. Ask one of them for a grant, a token or a change
to the instance's setup. Any authenticated token may ask, also one with no
grant yet; needs smeldr.dev/mcp with administrators (core T944) on the server.
`)
}
