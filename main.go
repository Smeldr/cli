// Package main is the forge-cli operator tool. It provides a terminal interface
// for managing content and tokens on a running Forge instance over HTTP.
//
// Configuration is loaded from environment variables, falling back to a
// .forge-cli.env file in the working directory:
//
//	FORGE_URL     — base URL of the running Forge instance (required)
//	FORGE_TOKEN   — bearer token with appropriate role (required)
//	FORGE_MCP_URL — MCP message endpoint (default: FORGE_URL/mcp/message)
//
// Usage:
//
//	forge-cli <type> <verb> [slug] [flags]
//	forge-cli token <verb> [args...]
//	forge-cli status
package main

import (
	"fmt"
	"os"
)

const cliVersion = "0.10.0"

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}
	switch os.Args[1] {
	case "-h", "--help", "help":
		printUsage(os.Stdout)
	case "-v", "--version", "version":
		fmt.Fprintln(os.Stdout, "forge-cli v"+cliVersion)
	case "init":
		runInit(os.Args[2:])
	case "status":
		runStatus(os.Args[2:])
	case "token":
		runTokenCommand(os.Args[2:])
	case "media":
		runMediaCommand(os.Args[2:])
	case "webhook":
		runWebhookCommand(os.Args[2:])
	case "preview":
		runPreviewCommand(os.Args[2:])
	case "social":
		runSocialCommand(os.Args[2:])
	case "block":
		runBlockCommand(os.Args[2:])
	case "audit":
		runAuditCommand(os.Args[2:])
	case "oauth":
		runOAuthCommand(os.Args[2:])
	default:
		runContentCommand(os.Args[1], os.Args[2:])
	}
}

func printUsage(w *os.File) {
	fmt.Fprintf(w, `forge-cli v%s — Forge operator CLI

Usage:
  forge-cli init [--url URL] [--bootstrap-token TOKEN]   bootstrap a new instance
  forge-cli <type> <verb> [slug] [flags]                 content operations
  forge-cli token <verb> [args]                          token management
  forge-cli webhook <verb> [args]                        webhook management
  forge-cli preview <prefix> <slug>                      generate draft preview URL
  forge-cli social <subcommand> [args]                   forge-social post, credential, and platform management
  forge-cli block <node|section|item> <verb> [args]      block system: nodes + composition (T32)
  forge-cli audit <subcommand> [args]                    audit trail (Editor role required)
  forge-cli status                                       connectivity check

Content verbs (type is the URL path segment, e.g. "posts", "doc-pages"):
  create    --from <file>                  create a new draft
  update    <slug> --from <file>           update fields (preserves absent fields)
  publish   <slug>                         transition to published
  unpublish <slug>                         revert published item to draft
  archive   <slug>                         transition to archived
  delete    <slug>                         permanently delete
  list      [--status draft|published|archived|scheduled]
  get       <slug>

Token verbs (Admin role required):
  create <name> <role> <ttl-days>          issue a new named token
  list                                     list all tokens
  revoke <id>                              revoke a token by fingerprint ID

Webhook verbs (Admin role required):
  create --url <URL> --events <e1,e2,...>  register a new endpoint
  list                                     list endpoints with delivery stats
  delete <id>                              permanently remove an endpoint
  deliveries --job <job-id>                show delivery logs for a job
  deliveries --endpoint <endpoint-id>      show all jobs for an endpoint
  retry <job-id>                           re-queue a dead-lettered job

Preview (Admin role required):
  preview <prefix> <slug>                  generate signed draft preview URL (12 h)

Media subcommands:
  upload <file> [--description <text>]     upload a file to the media library
  list [--type image|document|video|other] list media records
  delete <id>                              permanently delete a media record

Social subcommands:
  post create --credential <id> --body "..." [--platform mastodon|linkedin|x] [--at <RFC3339>]
  post list   [--status draft|scheduled|queued|published|archived|failed]
  post get|publish|archive|delete <id>
  credential create --platform mastodon|linkedin|x [--instance-url <url>]
  credential list
  credential get <id>
  credential delete <id>
  platform configure --platform mastodon|linkedin|x --client-id <id> --client-secret <secret> --redirect-url <url> [--instance-url <url>] [--success-url <url>]

Block subcommands (T32 — Fields keys are case-sensitive PascalCase):
  node create  --type <type_name> [--field K=V ...] [--fields <json>]   (Author)
  node update <id> [--field K=V ...] [--fields <json>]                  (Author)
  node get|publish|archive <id>                                         (Author)
  node list [--type <type_name>] [--status <s>] [--json]                (Author)
  section add|remove <parent_id> <child_id>                             (Editor)
  section reorder <parent_id> <child_id1,child_id2,...>                 (Editor)
  item add|remove <parent_id> <child_id>                               (Editor)
  item reorder <parent_id> <child_id1,child_id2,...>                   (Editor)

Audit subcommands (Editor role required):
  list [--from RFC3339] [--to RFC3339] [--type TYPE] [--actor ACTOR]

OAuth subcommands:
  revoke <token>                           revoke an OAuth refresh token (RFC 7009)

Environment variables:
  FORGE_URL      base URL of the running Forge instance (required)
  FORGE_TOKEN    bearer token with appropriate role (required)
  FORGE_MCP_URL  MCP message endpoint (default: FORGE_URL/mcp/message)

Configuration can also be stored in .forge-cli.env in the working directory.
`, cliVersion)
}
