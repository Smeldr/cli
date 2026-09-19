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
//	smeldr-cli status
package main
