package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// initBootstrapToken is the bootstrap token init spends (T961).
var initBootstrapToken = secretSource{
	flag:      "bootstrap-token",
	stdinFlag: "bootstrap-token-stdin",
	env:       "SMELDR_BOOTSTRAP_TOKEN",
	missing:   "init requires the bootstrap token from the startup log",
}

// runInit bootstraps a new Smeldr instance by using a bootstrap token to create
// a named admin token, then writes .smeldr-cli.env for subsequent CLI use.
//
// Usage:
//
//	SMELDR_BOOTSTRAP_TOKEN=... smeldr-cli init [--url URL] [--name NAME] [--days N] [--force]
func runInit(args []string) {
	fs := newCommandFlags("init")
	url := fs.String("url", "http://localhost:8080", "Base URL of the Smeldr instance")
	bootstrapFlag, bootstrapStdin := initBootstrapToken.secretFlags(fs, "the bootstrap token from the startup log")
	name := fs.String("name", "operator", "Name for the created admin token")
	days := fs.Int("days", 365, "Token TTL in days")
	force := fs.Bool("force", false, "Overwrite existing .smeldr-cli.env")
	noApprover := fs.Bool("no-approver", false, "grant admin only: the token cannot ratify Decisions")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: SMELDR_BOOTSTRAP_TOKEN=... smeldr-cli init [--url URL] [--name NAME] [--days N] [--force] [--no-approver]")
		fmt.Fprintln(os.Stderr, "   or: smeldr-cli init --bootstrap-token-stdin [...] < token.txt")
		fmt.Fprintln(os.Stderr, "Creates your token (class human, admin and approver: it can ratify Decisions) and writes .smeldr-cli.env.")
		fs.PrintDefaults()
	}
	parseCommandFlags(fs, args, 0, "")

	bootstrapToken, err := initBootstrapToken.read(fs, *bootstrapFlag, *bootstrapStdin)
	if err != nil {
		fatal("init: %v", err)
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

	// Refuse an existing env file before anything is created, so a refused
	// init leaves no orphaned token behind.
	const envFile = ".smeldr-cli.env"
	if _, err := os.Stat(envFile); err == nil && !*force {
		fatal("%s already exists — use --force to overwrite", envFile)
	}

	// Step 2: decide the grants. The person running init with the bootstrap
	// token founds the organization (D142 point 6): admin and approver,
	// unless --no-approver, or a global approver already exists (T970).
	tmpCfg := Config{
		ForgeURL: instanceURL,
		Token:    bootstrapToken,
		MCPURL:   instanceURL + "/mcp/message",
	}
	governed, approverExists, err := initGovernance(tmpCfg)
	if err != nil {
		fatal("init: %v", err)
	}
	founder := governed && !*noApprover && !approverExists

	// Step 3: create the person's token, class human, via MCP.
	text, err := mcpCall(tmpCfg, "create_token", map[string]any{
		"name":            *name,
		"role":            "admin",
		"expires_in_days": float64(*days),
		"actor_class":     "human",
		"reason":          initTokenReason,
	})
	if err != nil {
		fatal("create_token failed: %v", err)
	}
	var result map[string]any
	// Never print the response: it holds the new admin token (T961).
	if json.Unmarshal([]byte(text), &result) != nil {
		fatal("create_token returned a response that is not JSON (%d bytes); the token, if one was made, is not shown: revoke tokens named %q and run init again", len(text), *name)
	}
	newToken, ok := result["token"].(string)
	if !ok || newToken == "" {
		fatal("create_token returned no token")
	}
	tokenID, _ := result["token_id"].(string)
	fmt.Printf("OK  token %q created (class human, admin, %d days)\n", *name, *days)

	// Step 4: the grants. Under governance a token's role field grants
	// nothing; without them the token could not act as admin (T970).
	switch {
	case !governed:
		fmt.Println("OK  no governance on this server: the token's role is all it needs")
	case tokenID == "":
		fatal("create_token returned no token_id, so token %q holds no grant: revoke it and run init again", *name)
	default:
		roles := []string{"admin"}
		if founder {
			roles = append(roles, "approver")
		}
		for _, role := range roles {
			if _, err := mcpCall(tmpCfg, "grant_role", map[string]any{"token_id": tokenID, "role": role, "reason": initFounderReason}); err != nil {
				fatal("grant_role %s for token %q failed: %v (revoke the token and run init again)", role, *name, err)
			}
		}
		switch {
		case founder:
			fmt.Println("OK  grants: admin, approver (founder of the organization, D142): this token can ratify Decisions")
		case *noApprover:
			fmt.Println("OK  grants: admin (--no-approver: this token cannot ratify)")
		default:
			fmt.Println("OK  grants: admin (a global approver already exists, so no approver grant: ask an administrator for one)")
		}
	}

	// Step 5: write .smeldr-cli.env with the new SMELDR_* variable names (T86).
	content := fmt.Sprintf("SMELDR_URL=%s\nSMELDR_TOKEN=%s\n", instanceURL, newToken)
	if err := os.WriteFile(envFile, []byte(content), 0o600); err != nil {
		fatal("write %s: %v", envFile, err)
	}
	// WriteFile keeps an existing file's mode (--force): set 0600 again.
	if err := os.Chmod(envFile, 0o600); err != nil {
		fatal("chmod %s: %v", envFile, err)
	}
	fmt.Printf("OK  %s written (mode 600; keep it out of version control: add it to .gitignore)\n", envFile)

	// Step 6: verify the new token can act as admin, not only reach the
	// server: list_tokens needs the admin grant.
	verifyCfg := Config{
		ForgeURL: instanceURL,
		Token:    newToken,
		MCPURL:   instanceURL + "/mcp/message",
	}
	if _, err := mcpCall(verifyCfg, "list_tokens", map[string]any{}); err != nil {
		fmt.Fprintf(os.Stderr, "WARN  the new token cannot act as admin (env file written; investigate before use): %v\n", err)
		return
	}
	fmt.Println("OK  verification passed: the new token acts as admin")
}

// The reasons init records: the token is the person's, and its grants are
// the founder's (D142 point 6), in core's own words.
const (
	initTokenReason   = "created by smeldr-cli init for the person setting up this server"
	initFounderReason = "founder of the organization (D142)"
)

// initGovernance reports whether the server has governance (grant_role and
// list_grants exist) and whether an unscoped approver grant already exists,
// read with the bootstrap token.
func initGovernance(cfg Config) (governed, approverExists bool, err error) {
	text, err := mcpCall(cfg, "list_grants", map[string]any{})
	if err != nil {
		if strings.Contains(err.Error(), "unknown tool") {
			return false, false, nil
		}
		return false, false, fmt.Errorf("list_grants: %w", err)
	}
	var res struct {
		Grants []struct {
			RoleName      string     `json:"RoleName"`
			ScopeAnchorID string     `json:"ScopeAnchorID"`
			ScopeStatic   []string   `json:"ScopeStatic"`
			ExpiresAt     *time.Time `json:"ExpiresAt"`
		} `json:"grants"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		return true, false, fmt.Errorf("list_grants: unexpected response: %w", err)
	}
	for _, g := range res.Grants {
		live := g.ExpiresAt == nil || g.ExpiresAt.After(time.Now())
		if g.RoleName == "approver" && g.ScopeAnchorID == "" && len(g.ScopeStatic) == 0 && live {
			return true, true, nil
		}
	}
	return true, false, nil
}
