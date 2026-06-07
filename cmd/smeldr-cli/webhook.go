package main

import (
	"fmt"
	"os"
	"strings"
)

// runWebhookCommand dispatches webhook subcommands. args begins with the verb.
func runWebhookCommand(args []string) {
	if len(args) == 0 {
		printWebhookHelp()
		os.Exit(1)
	}
	switch args[0] {
	case "-h", "--help", "help":
		printWebhookHelp()
	case "create":
		runWebhookCreate(args[1:])
	case "list":
		runWebhookList(args[1:])
	case "delete":
		runWebhookDelete(args[1:])
	case "deliveries":
		runWebhookDeliveries(args[1:])
	case "retry":
		runWebhookRetry(args[1:])
	default:
		fatal("unknown webhook verb %q — use: create list delete deliveries retry", args[0])
	}
}

func printWebhookHelp() {
	fmt.Fprint(os.Stdout, `smeldr-cli webhook — outbound webhook management (Admin role required)

Verbs:
  create --url <URL> --events <e1,e2,...>  register a new endpoint
  list                                     list endpoints with delivery stats
  delete <id>                              permanently remove an endpoint
  deliveries --job <job-id>                show delivery logs for a job
  deliveries --endpoint <endpoint-id>      show all jobs for an endpoint
  retry <job-id>                           re-queue a dead-lettered job

Event names follow the pattern <type>.<lifecycle>, e.g.:
  post.created  post.updated  post.published  post.archived  post.deleted

The MCP endpoint is used for webhook operations (SMELDR_MCP_URL).
`)
}

// runWebhookCreate registers a new webhook endpoint via create_webhook MCP tool.
func runWebhookCreate(args []string) {
	var urlFlag, eventsFlag string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--url":
			if i+1 < len(args) {
				urlFlag = args[i+1]
				i++
			}
		case "--events":
			if i+1 < len(args) {
				eventsFlag = args[i+1]
				i++
			}
		}
	}
	if urlFlag == "" {
		fatal("webhook create requires --url <https://...>")
	}
	if eventsFlag == "" {
		fatal("webhook create requires --events <event1,event2,...>")
	}
	parts := strings.Split(eventsFlag, ",")
	events := make([]any, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			events = append(events, p)
		}
	}
	if len(events) == 0 {
		fatal("webhook create --events must be a comma-separated list of event names")
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}

	text, err := mcpCall(cfg, "create_webhook", map[string]any{
		"url":    urlFlag,
		"events": events,
	})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// runWebhookList lists all webhook endpoints via list_webhooks MCP tool.
func runWebhookList(args []string) {
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	_ = args // no flags
	text, err := mcpCall(cfg, "list_webhooks", map[string]any{})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// runWebhookDelete permanently removes a webhook endpoint via delete_webhook MCP tool.
func runWebhookDelete(args []string) {
	if len(args) == 0 {
		fatal("webhook delete requires a webhook endpoint ID")
	}
	id := args[0]

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}

	text, err := mcpCall(cfg, "delete_webhook", map[string]any{"id": id})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// runWebhookDeliveries shows delivery logs for a job or all jobs for an endpoint.
func runWebhookDeliveries(args []string) {
	var jobID, endpointID string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--job":
			if i+1 < len(args) {
				jobID = args[i+1]
				i++
			}
		case "--endpoint":
			if i+1 < len(args) {
				endpointID = args[i+1]
				i++
			}
		}
	}
	if jobID == "" && endpointID == "" {
		fatal("webhook deliveries requires --job <job-id> or --endpoint <endpoint-id>")
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}

	callArgs := map[string]any{}
	if jobID != "" {
		callArgs["job_id"] = jobID
	} else {
		callArgs["endpoint_id"] = endpointID
	}
	text, err := mcpCall(cfg, "list_webhook_deliveries", callArgs)
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// runWebhookRetry re-queues a dead-lettered webhook job via retry_webhook MCP tool.
func runWebhookRetry(args []string) {
	if len(args) == 0 {
		fatal("webhook retry requires a job ID")
	}
	jobID := args[0]

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}

	text, err := mcpCall(cfg, "retry_webhook", map[string]any{"job_id": jobID})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}
