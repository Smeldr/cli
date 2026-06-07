package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// runSocialCommand dispatches social subcommands. args begins after "social".
func runSocialCommand(args []string) {
	if len(args) == 0 {
		printSocialHelp()
		os.Exit(1)
	}
	switch args[0] {
	case "-h", "--help", "help":
		printSocialHelp()
	case "post":
		runSocialPostCommand(args[1:])
	case "credential":
		runSocialCredentialCommand(args[1:])
	case "schedule":
		runSocialScheduleCommand(args[1:])
	case "platform":
		runSocialPlatformCommand(args[1:])
	default:
		fatal("unknown social subcommand %q — use: post credential schedule platform", args[0])
	}
}

func printSocialHelp() {
	fmt.Fprint(os.Stdout, `smeldr-cli social — social management

Subcommands:
  post        <verb> [args]   manage scheduled social posts
  credential  <verb> [args]   manage platform OAuth credentials
  schedule    <verb> [args]   manage publication schedules
  platform    <verb> [args]   manage per-platform OAuth app configuration

Post verbs:
  create  --credential <id> --body "..." [--platform mastodon|linkedin|x] [--at <RFC3339>]
  queue   --credential <id> --body "..." [--platform mastodon|linkedin|x]
  list    [--status draft|scheduled|queued|published|archived|failed]
  get     <id>
  publish <id>
  archive <id>
  delete  <id>

Credential verbs:
  create  --platform mastodon|linkedin|x [--instance-url <url>]
  list
  get     <id>
  delete  <id>

Schedule verbs:
  create  --credential <id> --slot "<weekday> HH:MM IANA/TZ" [--slot ...]
  show    --credential <id>
  pause   --credential <id>
  resume  --credential <id>
  delete  --credential <id>

Platform verbs:
  configure  --platform mastodon|linkedin|x --client-id <id> --client-secret <secret> --redirect-url <url> [--instance-url <url>] [--success-url <url>]

The MCP endpoint is used for all social operations (SMELDR_MCP_URL).
`)
}

// ─── post subcommands ──────────────────────────────────────────────────────────

func runSocialPostCommand(args []string) {
	if len(args) == 0 {
		printSocialPostHelp()
		os.Exit(1)
	}
	switch args[0] {
	case "-h", "--help", "help":
		printSocialPostHelp()
	case "create":
		runSocialPostCreate(args[1:])
	case "list":
		runSocialPostList(args[1:])
	case "get":
		runSocialPostGet(args[1:])
	case "publish":
		runSocialPostPublish(args[1:])
	case "archive":
		runSocialPostArchive(args[1:])
	case "queue":
		runSocialPostQueue(args[1:])
	case "delete":
		runSocialPostDelete(args[1:])
	default:
		fatal("unknown post verb %q — use: create queue list get publish archive delete", args[0])
	}
}

func printSocialPostHelp() {
	fmt.Fprint(os.Stdout, `smeldr-cli social post — scheduled post management

Verbs:
  create  --credential <id> --body "..." [--platform mastodon|linkedin|x] [--at <RFC3339>]
  queue   --credential <id> --body "..." [--platform mastodon|linkedin|x]
  list    [--status draft|scheduled|queued|published|archived|failed]
  get     <id>
  publish <id>
  archive <id>
  delete  <id>
`)
}

func runSocialPostCreate(args []string) {
	var platform, credential, body, at string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--platform":
			if i+1 < len(args) {
				platform = args[i+1]
				i++
			}
		case "--credential":
			if i+1 < len(args) {
				credential = args[i+1]
				i++
			}
		case "--body":
			if i+1 < len(args) {
				body = args[i+1]
				i++
			}
		case "--at":
			if i+1 < len(args) {
				at = args[i+1]
				i++
			}
		}
	}
	if credential == "" {
		fatal("social post create requires --credential <id>")
	}
	if body == "" {
		fatal("social post create requires --body \"...\"")
	}
	if platform == "" {
		platform = "mastodon"
	}

	params := map[string]any{
		"platform":      platform,
		"credential_id": credential,
		"body":          body,
	}
	if at != "" {
		params["scheduled_at"] = at
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "create_scheduled_post", params)
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func runSocialPostList(args []string) {
	var status string
	for i := 0; i < len(args); i++ {
		if args[i] == "--status" && i+1 < len(args) {
			status = args[i+1]
			i++
		}
	}

	params := map[string]any{}
	if status != "" {
		params["status"] = status
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "list_scheduled_posts", params)
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func runSocialPostGet(args []string) {
	if len(args) == 0 {
		fatal("social post get requires <id>")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "get_scheduled_post", map[string]any{"slug": args[0]})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func runSocialPostPublish(args []string) {
	if len(args) == 0 {
		fatal("social post publish requires <id>")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "publish_scheduled_post", map[string]any{"slug": args[0]})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func runSocialPostArchive(args []string) {
	if len(args) == 0 {
		fatal("social post archive requires <id>")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "archive_scheduled_post", map[string]any{"slug": args[0]})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func runSocialPostDelete(args []string) {
	if len(args) == 0 {
		fatal("social post delete requires <id>")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "delete_scheduled_post", map[string]any{"slug": args[0]})
	if err != nil {
		fatal("%v", err)
	}
	if text != "" {
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
	}
}

// ─── credential subcommands ────────────────────────────────────────────────────

func runSocialCredentialCommand(args []string) {
	if len(args) == 0 {
		printSocialCredentialHelp()
		os.Exit(1)
	}
	switch args[0] {
	case "-h", "--help", "help":
		printSocialCredentialHelp()
	case "create":
		runSocialCredentialCreate(args[1:])
	case "list":
		runSocialCredentialList(args[1:])
	case "get":
		runSocialCredentialGet(args[1:])
	case "delete":
		runSocialCredentialDelete(args[1:])
	default:
		fatal("unknown credential verb %q — use: create list get delete", args[0])
	}
}

func printSocialCredentialHelp() {
	fmt.Fprint(os.Stdout, `smeldr-cli social credential — platform OAuth credential management

Verbs:
  create  --platform mastodon|linkedin|x [--instance-url <url>]
  list
  get     <id>
  delete  <id>
`)
}

func runSocialCredentialCreate(args []string) {
	var platform, instanceURL string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--platform":
			if i+1 < len(args) {
				platform = args[i+1]
				i++
			}
		case "--instance-url":
			if i+1 < len(args) {
				instanceURL = args[i+1]
				i++
			}
		}
	}
	if platform == "" {
		fatal("social credential create requires --platform mastodon|linkedin|x")
	}
	if instanceURL != "" && platform == "x" {
		fatal("--instance-url is not supported for platform x")
	}

	params := map[string]any{"platform": platform}
	if instanceURL != "" {
		params["instance_url"] = instanceURL
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "create_social_credential", params)
	if err != nil {
		fatal("%v", err)
	}

	// create_social_credential returns {"redirect_url": "...", "message": "..."}.
	// Print the URL prominently so the operator can open it immediately.
	var result struct {
		RedirectURL string `json:"redirect_url"`
		Message     string `json:"message"`
	}
	if jsonErr := json.Unmarshal([]byte(text), &result); jsonErr == nil && result.RedirectURL != "" {
		fmt.Println("Open this URL in your browser to connect your account:")
		fmt.Println(result.RedirectURL)
		return
	}
	// Fallback: print raw JSON.
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func runSocialCredentialList(args []string) {
	_ = args
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "list_social_credentials", map[string]any{})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func runSocialCredentialGet(args []string) {
	if len(args) == 0 {
		fatal("social credential get requires <id>")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "get_social_credential", map[string]any{"slug": args[0]})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func runSocialCredentialDelete(args []string) {
	if len(args) == 0 {
		fatal("social credential delete requires <id>")
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "delete_social_credential", map[string]any{"slug": args[0]})
	if err != nil {
		fatal("%v", err)
	}
	if text != "" {
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
	}
}

// ─── post queue verb ───────────────────────────────────────────────────────────

func runSocialPostQueue(args []string) {
	var platform, credential, body string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--platform":
			if i+1 < len(args) {
				platform = args[i+1]
				i++
			}
		case "--credential":
			if i+1 < len(args) {
				credential = args[i+1]
				i++
			}
		case "--body":
			if i+1 < len(args) {
				body = args[i+1]
				i++
			}
		}
	}
	if credential == "" {
		fatal("social post queue requires --credential <id>")
	}
	if body == "" {
		fatal("social post queue requires --body \"...\"")
	}
	if platform == "" {
		platform = "mastodon"
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "create_scheduled_post", map[string]any{
		"platform":      platform,
		"credential_id": credential,
		"body":          body,
		"status":        "queued",
	})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// ─── schedule subcommands ──────────────────────────────────────────────────────

func runSocialScheduleCommand(args []string) {
	if len(args) == 0 {
		printSocialScheduleHelp()
		os.Exit(1)
	}
	switch args[0] {
	case "-h", "--help", "help":
		printSocialScheduleHelp()
	case "create":
		runSocialScheduleCreate(args[1:])
	case "show":
		runSocialScheduleShow(args[1:])
	case "pause":
		runSocialSchedulePause(args[1:])
	case "resume":
		runSocialScheduleResume(args[1:])
	case "delete":
		runSocialScheduleDelete(args[1:])
	default:
		fatal("unknown schedule verb %q — use: create show pause resume delete", args[0])
	}
}

func printSocialScheduleHelp() {
	fmt.Fprint(os.Stdout, `smeldr-cli social schedule — publication schedule management

Verbs:
  create  --credential <id> --slot "<weekday> HH:MM IANA/TZ" [--slot ...]
  show    --credential <id>
  pause   --credential <id>
  resume  --credential <id>
  delete  --credential <id>

Slot format: "<weekday> <HH:MM> <IANA timezone>"
  Example: "monday 09:00 Europe/Copenhagen"
  Weekdays: sunday monday tuesday wednesday thursday friday saturday
`)
}

func runSocialScheduleCreate(args []string) {
	var credential string
	var slotArgs []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--credential":
			if i+1 < len(args) {
				credential = args[i+1]
				i++
			}
		case "--slot":
			if i+1 < len(args) {
				slotArgs = append(slotArgs, args[i+1])
				i++
			}
		}
	}
	if credential == "" {
		fatal("social schedule create requires --credential <id>")
	}
	if len(slotArgs) == 0 {
		fatal("social schedule create requires at least one --slot")
	}

	slots := make([]map[string]any, 0, len(slotArgs))
	for _, s := range slotArgs {
		slot, err := parseSlot(s)
		if err != nil {
			fatal("invalid --slot %q: %v", s, err)
		}
		slots = append(slots, slot)
	}
	slotsJSON, err := json.Marshal(slots)
	if err != nil {
		fatal("failed to encode slots: %v", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "create_publication_schedule", map[string]any{
		"credential_id": credential,
		"slots":         string(slotsJSON),
	})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func runSocialScheduleShow(args []string) {
	credential := requireCredentialFlag(args, "social schedule show")
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	id, err := findScheduleByCredential(cfg, credential)
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "get_publication_schedule", map[string]any{"slug": id})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func runSocialSchedulePause(args []string) {
	credential := requireCredentialFlag(args, "social schedule pause")
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	id, err := findScheduleByCredential(cfg, credential)
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "update_publication_schedule", map[string]any{
		"slug":   id,
		"status": "paused",
	})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func runSocialScheduleResume(args []string) {
	credential := requireCredentialFlag(args, "social schedule resume")
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	id, err := findScheduleByCredential(cfg, credential)
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "update_publication_schedule", map[string]any{
		"slug":   id,
		"status": "active",
	})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func runSocialScheduleDelete(args []string) {
	credential := requireCredentialFlag(args, "social schedule delete")
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	id, err := findScheduleByCredential(cfg, credential)
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "delete_publication_schedule", map[string]any{"slug": id})
	if err != nil {
		fatal("%v", err)
	}
	if text != "" {
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
	}
}

// requireCredentialFlag parses --credential from args and fatals if absent.
func requireCredentialFlag(args []string, cmd string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--credential" && i+1 < len(args) {
			return args[i+1]
		}
	}
	fatal("%s requires --credential <id>", cmd)
	return ""
}

// findScheduleByCredential calls list_publication_schedules and returns the
// schedule ID for the given credential_id, or an error if not found.
func findScheduleByCredential(cfg Config, credentialID string) (string, error) {
	text, err := mcpCall(cfg, "list_publication_schedules", map[string]any{})
	if err != nil {
		return "", err
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		return "", fmt.Errorf("unexpected response from list_publication_schedules: %w", err)
	}
	for _, item := range items {
		if item["credential_id"] == credentialID {
			id, _ := item["id"].(string)
			if id != "" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("no schedule found for credential %q", credentialID)
}

// ─── platform subcommands ─────────────────────────────────────────────────────

func runSocialPlatformCommand(args []string) {
	if len(args) == 0 {
		printSocialPlatformHelp()
		os.Exit(1)
	}
	switch args[0] {
	case "-h", "--help", "help":
		printSocialPlatformHelp()
	case "configure":
		runSocialPlatformConfigure(args[1:])
	default:
		fatal("unknown platform verb %q — use: configure", args[0])
	}
}

func printSocialPlatformHelp() {
	fmt.Fprint(os.Stdout, `smeldr-cli social platform — per-platform OAuth app configuration

Verbs:
  configure  --platform mastodon|linkedin|x \
             --client-id <id> \
             --client-secret <secret> \
             --redirect-url <url> \
             [--instance-url <url>]  (mastodon only) \
             [--success-url <url>]
`)
}

func runSocialPlatformConfigure(args []string) {
	var platform, clientID, clientSecret, redirectURL, instanceURL, successURL string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--platform":
			if i+1 < len(args) {
				platform = args[i+1]
				i++
			}
		case "--client-id":
			if i+1 < len(args) {
				clientID = args[i+1]
				i++
			}
		case "--client-secret":
			if i+1 < len(args) {
				clientSecret = args[i+1]
				i++
			}
		case "--redirect-url":
			if i+1 < len(args) {
				redirectURL = args[i+1]
				i++
			}
		case "--instance-url":
			if i+1 < len(args) {
				instanceURL = args[i+1]
				i++
			}
		case "--success-url":
			if i+1 < len(args) {
				successURL = args[i+1]
				i++
			}
		}
	}
	if platform == "" {
		fatal("social platform configure requires --platform mastodon|linkedin|x")
	}
	if clientID == "" {
		fatal("social platform configure requires --client-id <id>")
	}
	if clientSecret == "" {
		fatal("social platform configure requires --client-secret <secret>")
	}
	if redirectURL == "" {
		fatal("social platform configure requires --redirect-url <url>")
	}
	if instanceURL != "" && platform != "mastodon" {
		fatal("--instance-url is only supported for platform mastodon")
	}

	params := map[string]any{
		"platform":      platform,
		"client_id":     clientID,
		"client_secret": clientSecret,
		"redirect_url":  redirectURL,
	}
	if instanceURL != "" {
		params["instance_url"] = instanceURL
	}
	if successURL != "" {
		params["success_url"] = successURL
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "create_platform_config", params)
	if err != nil {
		fatal("%v", err)
	}
	// Platform config is stored — never echo credentials back.
	// Print a minimal confirmation from the server response if present.
	var result struct {
		Message string `json:"message"`
	}
	if jsonErr := json.Unmarshal([]byte(text), &result); jsonErr == nil && result.Message != "" {
		fmt.Println(result.Message)
		return
	}
	fmt.Printf("Platform %q configured.\n", platform)
}

// parseSlot parses a slot string of the form "<weekday> <HH:MM> <IANA timezone>".
// weekday is case-insensitive (e.g. "monday", "Monday").
func parseSlot(s string) (map[string]any, error) {
	parts := strings.Fields(s)
	if len(parts) != 3 {
		return nil, fmt.Errorf("expected \"<weekday> <HH:MM> <IANA timezone>\", got %q", s)
	}
	weekdayNames := map[string]int{
		"sunday": 0, "monday": 1, "tuesday": 2, "wednesday": 3,
		"thursday": 4, "friday": 5, "saturday": 6,
	}
	weekday, ok := weekdayNames[strings.ToLower(parts[0])]
	if !ok {
		return nil, fmt.Errorf("unknown weekday %q — use: sunday monday tuesday wednesday thursday friday saturday", parts[0])
	}
	var h, m int
	if _, err := fmt.Sscanf(parts[1], "%d:%d", &h, &m); err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return nil, fmt.Errorf("time must be HH:MM (24-hour), got %q", parts[1])
	}
	if _, err := time.LoadLocation(parts[2]); err != nil {
		return nil, fmt.Errorf("timezone %q is not a valid IANA name", parts[2])
	}
	return map[string]any{
		"weekday":  weekday,
		"time":     parts[1],
		"timezone": parts[2],
	}, nil
}
