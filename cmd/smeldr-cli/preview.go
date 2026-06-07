package main

import (
	"fmt"
	"os"
	"strings"
)

// runPreviewCommand generates a signed draft preview URL via the
// create_preview_url MCP tool and prints it to stdout.
//
// Usage:
//
//	smeldr-cli preview <prefix> <slug>
//
// prefix must include the leading slash (e.g. "/posts").
// The full preview URL is printed to stdout.
func runPreviewCommand(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printPreviewHelp()
		if len(args) == 0 {
			os.Exit(1)
		}
		return
	}
	if len(args) < 2 {
		fatal("preview requires <prefix> and <slug> — e.g. smeldr-cli preview /posts my-draft")
	}

	prefix := args[0]
	slug := args[1]

	if !strings.HasPrefix(prefix, "/") {
		fatal("prefix must begin with / (e.g. /posts) — got %q", prefix)
	}
	if slug == "" {
		fatal("slug must not be empty")
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("preview: %v", err)
	}
	url, err := mcpCall(cfg, "create_preview_url", map[string]any{
		"prefix": prefix,
		"slug":   slug,
	})
	if err != nil {
		fatal("preview: %v", err)
	}
	// mcpCall returns the JSON-decoded text content; strip surrounding quotes
	// that may appear when the server returns a plain JSON string value.
	url = strings.Trim(url, `"`)
	fmt.Fprintln(os.Stdout, url)
}

func printPreviewHelp() {
	fmt.Fprint(os.Stdout, `smeldr-cli preview — generate a signed draft preview URL (Admin role required)

Usage:
  smeldr-cli preview <prefix> <slug>

Arguments:
  prefix   URL prefix of the content module (e.g. /posts, /docs). Must include the leading slash.
  slug     Slug of the draft item to preview (e.g. my-draft-post).

The preview URL bypasses Published-only visibility for the token lifetime (default 12 h).
The full URL is printed to stdout and can be shared with reviewers without a login.

Examples:
  smeldr-cli preview /posts my-draft-post
  smeldr-cli preview /docs getting-started-draft

The MCP endpoint is used for this operation (SMELDR_MCP_URL).
`)
}
