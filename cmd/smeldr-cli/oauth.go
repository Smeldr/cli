package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func runOAuthCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "smeldr-cli oauth: expected subcommand (revoke)")
		os.Exit(1)
	}
	switch args[0] {
	case "-h", "--help", "help":
		printOAuthHelp()
	case "revoke":
		runOAuthRevoke(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "smeldr-cli oauth: unknown subcommand %q\n", args[0])
		os.Exit(1)
	}
}

// printOAuthHelp is smeldr-cli oauth --help (T1147).
func printOAuthHelp() {
	fmt.Print(`smeldr-cli oauth: OAuth tokens issued by smeldr.dev/oauth

Usage:
  smeldr-cli oauth revoke <token>   revoke one OAuth access or refresh token

Needs SMELDR_URL and an Admin token in SMELDR_TOKEN, and a server with OAUTH_ISSUER set.
`)
}

func runOAuthRevoke(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: smeldr-cli oauth revoke <token>")
		os.Exit(1)
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}

	resp, err := http.PostForm(cfg.ForgeURL+"/oauth/revoke", url.Values{"token": {args[0]}})
	if err != nil {
		fatal("request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		fatal("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	fmt.Println("Token revoked.")
}
