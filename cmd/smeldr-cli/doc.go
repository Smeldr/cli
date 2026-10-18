// Package main is the smeldr-cli operator tool. It provides a terminal interface
// for managing content and tokens on a running Smeldr instance over HTTP.
//
// Configuration is loaded from environment variables, falling back to a
// .smeldr-cli.env file in the working directory:
//
//	SMELDR_URL     — base URL of the running Smeldr instance (required)
//	SMELDR_TOKEN   — bearer token with appropriate role (required)
//	SMELDR_MCP_URL — MCP message endpoint (default: SMELDR_URL/mcp/message)
//
// Usage:
//
//	smeldr-cli <type> <verb> [slug] [flags]
//	smeldr-cli token <verb> [args...]
//	smeldr-cli subscription <verb> [args...]
//	smeldr-cli signal <answer|withdraw> <address> [flags]
//	smeldr-cli search [<text>] [--type t] [--state s] [--place p|none]
//	smeldr-cli watch --subscription <address>
//	smeldr-cli status
//
// watch prints the event stream, one JSON line per event, and reconnects with
// ?after=<last event id> when the connection drops or closes, so it can serve
// as a long-running listener.
package main
