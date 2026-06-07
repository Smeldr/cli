package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// runInit bootstraps a new Smeldr instance by using a bootstrap token to create
// a named admin token, then writes .smeldr-cli.env for subsequent CLI use.
//
// Usage:
//
//	smeldr-cli init [--url URL] [--bootstrap-token TOKEN] [--name NAME] [--days N] [--force]
func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	url := fs.String("url", "http://localhost:8080", "Base URL of the Forge instance")
	bootstrapToken := fs.String("bootstrap-token", "", "Bootstrap token from startup log (required)")
	name := fs.String("name", "operator", "Name for the created admin token")
	days := fs.Int("days", 365, "Token TTL in days")
	force := fs.Bool("force", false, "Overwrite existing .smeldr-cli.env")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: smeldr-cli init [--url URL] [--bootstrap-token TOKEN] [--name NAME] [--days N] [--force]")
		fs.PrintDefaults()
	}
	fs.Parse(args) //nolint:errcheck

	if *bootstrapToken == "" {
		fatal("--bootstrap-token is required (copy from startup log)")
	}
	if *days <= 0 {
		fatal("--days must be a positive integer")
	}

	instanceURL := strings.TrimRight(*url, "/")

	// Step 1: unauthenticated reachability check.
	resp, err := http.Get(instanceURL + "/_health") //nolint:noctx
	if err != nil {
		fatal("cannot reach %s: %v", instanceURL, err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		fatal("/_health returned %d — is the Smeldr instance running at %s?", resp.StatusCode, instanceURL)
	}
	fmt.Printf("OK  connected to %s\n", instanceURL)

	// Step 2: create a named admin token via MCP using the bootstrap token.
	tmpCfg := Config{
		ForgeURL: instanceURL,
		Token:    *bootstrapToken,
		MCPURL:   instanceURL + "/mcp/message",
	}
	text, err := mcpCall(tmpCfg, "create_token", map[string]any{
		"name":            *name,
		"role":            "admin",
		"expires_in_days": float64(*days),
	})
	if err != nil {
		fatal("create_token failed: %v", err)
	}

	// Step 3: extract "token" from the JSON result.
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		fatal("unexpected create_token response: %v\nraw: %s", err, text)
	}
	newToken, ok := result["token"].(string)
	if !ok || newToken == "" {
		fatal("create_token response missing token field\nraw: %s", text)
	}
	fmt.Printf("OK  token %q created (admin, %d days)\n", *name, *days)

	// Step 4: guard against overwriting an existing env file.
	const envFile = ".smeldr-cli.env"
	if _, err := os.Stat(envFile); err == nil && !*force {
		fatal("%s already exists — use --force to overwrite", envFile)
	}

	// Step 5: write .smeldr-cli.env with the new SMELDR_* variable names (T86).
	content := fmt.Sprintf("SMELDR_URL=%s\nSMELDR_TOKEN=%s\n", instanceURL, newToken)
	if err := os.WriteFile(envFile, []byte(content), 0o600); err != nil {
		fatal("write %s: %v", envFile, err)
	}
	fmt.Printf("OK  %s written\n", envFile)

	// Step 6: verify the new token works.
	verifyCfg := Config{
		ForgeURL: instanceURL,
		Token:    newToken,
		MCPURL:   instanceURL + "/mcp/message",
	}
	_, code, verifyErr := request(verifyCfg, http.MethodGet, instanceURL+"/_health", nil)
	if verifyErr != nil || code >= 400 {
		fmt.Fprintf(os.Stderr, "WARN  verification failed (env file written — investigate before use): %v\n", verifyErr)
		return
	}
	fmt.Println("OK  verification passed")
}
