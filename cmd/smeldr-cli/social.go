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
  list    [--status draft|scheduled|queued|published|archived|failed] [--json] [--fields <a,b,c>]
          aligned table by default (columns: id, platform, status, scheduled_at)
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

Post commands use SMELDR_URL + bearer token (SMELDR_TOKEN).
Credential/schedule/platform commands use the MCP endpoint (SMELDR_MCP_URL).
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
  list    [--status draft|scheduled|queued|published|archived|failed] [--json] [--fields <a,b,c>]
          aligned table by default (columns: id, platform, status, scheduled_at)
  get     <id>
  publish <id>
  archive <id>
  delete  <id>
`)
}

func runSocialPostCreate(args []string) {
	fs := newCommandFlags("social post create")
	platformF := fs.String("platform", "", "mastodon, linkedin or x (default mastodon)")
	credentialF := fs.String("credential", "", "the credential")
	bodyF := fs.String("body", "", "the text")
	atF := fs.String("at", "", "when, RFC3339")
	parseCommandFlags(fs, args, 0, "")
	platform, credential, body, at := *platformF, *credentialF, *bodyF, *atF
	if credential == "" {
		fatal("social post create requires --credential <id>")
	}
	if body == "" {
		fatal("social post create requires --body \"...\"")
	}
	if platform == "" {
		platform = "mastodon"
	}

	payload := map[string]any{
		"platform":      platform,
		"credential_id": credential,
		"body":          body,
	}
	if at != "" {
		payload["scheduled_at"] = at
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	raw, code, err := request(cfg, "POST", cfg.ForgeURL+"/social/posts", payload)
	if err != nil {
		fatal("%v", err)
	}
	if code >= 400 {
		fatal("server returned %d: %s", code, strings.TrimSpace(string(raw)))
	}
	if err := printJSON(raw); err != nil {
		fatal("%v", err)
	}
}

// defaultSocialPostListFields are the columns shown when post list is
// called without --fields.
var defaultSocialPostListFields = []string{"id", "platform", "status", "scheduled_at"}

func runSocialPostList(args []string) {
	fs := newCommandFlags("social post list")
	statusF := fs.String("status", "", "one status")
	fieldsF := fs.String("fields", "", "comma-separated columns")
	asJSON := fs.Bool("json", false, "raw JSON")
	parseCommandFlags(fs, args, 0, "")
	status, fields, jsonFlag := *statusF, *fieldsF, *asJSON

	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	url := cfg.ForgeURL + "/social/posts"
	if status != "" {
		url += "?status=" + status
	}
	raw, code, err := request(cfg, "GET", url, nil)
	if err != nil {
		fatal("%v", err)
	}
	if code >= 400 {
		fatal("server returned %d: %s", code, strings.TrimSpace(string(raw)))
	}

	if jsonFlag {
		if err := printJSON(raw); err != nil {
			fatal("%v", err)
		}
		return
	}

	var items []any
	if err := json.Unmarshal(raw, &items); err != nil {
		fatal("decode post list: %v", err)
	}
	cols := defaultSocialPostListFields
	if fields != "" {
		cols = splitFields(fields)
	}
	fmt.Print(keyedTable(items, cols, "No posts.\n"))
}

func runSocialPostGet(args []string) {
	id := oneArgument("social post get", args, "social post get requires <id>")
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	raw, code, err := request(cfg, "GET", cfg.ForgeURL+"/social/posts/"+id, nil)
	if err != nil {
		fatal("%v", err)
	}
	if code >= 400 {
		fatal("server returned %d: %s", code, strings.TrimSpace(string(raw)))
	}
	if err := printJSON(raw); err != nil {
		fatal("%v", err)
	}
}

func runSocialPostPublish(args []string) {
	id := oneArgument("social post publish", args, "social post publish requires <id>")
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	raw, code, err := request(cfg, "PUT", cfg.ForgeURL+"/social/posts/"+id, map[string]any{"status": "queued"})
	if err != nil {
		fatal("%v", err)
	}
	if code >= 400 {
		fatal("server returned %d: %s", code, strings.TrimSpace(string(raw)))
	}
	if err := printJSON(raw); err != nil {
		fatal("%v", err)
	}
}

func runSocialPostArchive(args []string) {
	id := oneArgument("social post archive", args, "social post archive requires <id>")
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	raw, code, err := request(cfg, "PUT", cfg.ForgeURL+"/social/posts/"+id, map[string]any{"status": "archived"})
	if err != nil {
		fatal("%v", err)
	}
	if code >= 400 {
		fatal("server returned %d: %s", code, strings.TrimSpace(string(raw)))
	}
	if err := printJSON(raw); err != nil {
		fatal("%v", err)
	}
}

func runSocialPostDelete(args []string) {
	id := oneArgument("social post delete", args, "social post delete requires <id>")
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	raw, code, err := request(cfg, "DELETE", cfg.ForgeURL+"/social/posts/"+id, nil)
	if err != nil {
		fatal("%v", err)
	}
	if code >= 400 {
		fatal("server returned %d: %s", code, strings.TrimSpace(string(raw)))
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
	fs := newCommandFlags("social credential create")
	platformF := fs.String("platform", "", "mastodon, linkedin or x")
	instanceF := fs.String("instance-url", "", "the Mastodon instance")
	parseCommandFlags(fs, args, 0, "")
	platform, instanceURL := *platformF, *instanceF
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
	id := oneArgument("social credential get", args, "social credential get requires <id>")
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "get_social_credential", map[string]any{"slug": id})
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

func runSocialCredentialDelete(args []string) {
	id := oneArgument("social credential delete", args, "social credential delete requires <id>")
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, "delete_social_credential", map[string]any{"slug": id})
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
	fs := newCommandFlags("social post queue")
	platformF := fs.String("platform", "", "mastodon, linkedin or x (default mastodon)")
	credentialF := fs.String("credential", "", "the credential")
	bodyF := fs.String("body", "", "the text")
	parseCommandFlags(fs, args, 0, "")
	platform, credential, body := *platformF, *credentialF, *bodyF
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
	raw, code, err := request(cfg, "POST", cfg.ForgeURL+"/social/posts", map[string]any{
		"platform":      platform,
		"credential_id": credential,
		"body":          body,
		"status":        "queued",
	})
	if err != nil {
		fatal("%v", err)
	}
	if code >= 400 {
		fatal("server returned %d: %s", code, strings.TrimSpace(string(raw)))
	}
	if err := printJSON(raw); err != nil {
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
	fs := newCommandFlags("social schedule create")
	credentialF := fs.String("credential", "", "the credential")
	var slotFlags stringsFlag
	fs.Var(&slotFlags, "slot", "a publication slot (repeatable)")
	parseCommandFlags(fs, args, 0, "")
	credential, slotArgs := *credentialF, []string(slotFlags)
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
	fs := newCommandFlags(cmd)
	credential := fs.String("credential", "", "the credential")
	parseCommandFlags(fs, args, 0, "")
	if *credential == "" {
		fatal("%s requires --credential <id>", cmd)
	}
	return *credential
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

// socialClientSecret is the OAuth app's client secret (T961).
var socialClientSecret = secretSource{
	flag:      "client-secret",
	stdinFlag: "client-secret-stdin",
	env:       "SMELDR_SOCIAL_CLIENT_SECRET",
	missing:   "social platform configure requires the app's client secret",
}

func runSocialPlatformConfigure(args []string) {
	fs := newCommandFlags("social platform configure")
	platformF := fs.String("platform", "", "mastodon, linkedin or x")
	clientIDF := fs.String("client-id", "", "the app's client ID")
	clientSecretF, secretStdin := socialClientSecret.secretFlags(fs, "the app's client secret")
	redirectF := fs.String("redirect-url", "", "the OAuth redirect URL")
	instanceF := fs.String("instance-url", "", "the Mastodon instance")
	successF := fs.String("success-url", "", "where to go after connecting")
	parseCommandFlags(fs, args, 0, "")
	platform, clientID := *platformF, *clientIDF
	redirectURL, instanceURL, successURL := *redirectF, *instanceF, *successF
	if platform == "" {
		fatal("social platform configure requires --platform mastodon|linkedin|x")
	}
	if clientID == "" {
		fatal("social platform configure requires --client-id <id>")
	}
	clientSecret, err := socialClientSecret.read(fs, *clientSecretF, *secretStdin)
	if err != nil {
		fatal("social platform configure: %v", err)
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
