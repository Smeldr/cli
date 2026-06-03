# Changelog — forge-cli

All notable changes to the `forge-cli` module are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.11.0] — 2026-06-03

### Changed (additive, non-breaking)

- **`SMELDR_*` env vars preferred (Amendment A123, T86 / T78):** `loadConfig` now
  reads `SMELDR_URL`, `SMELDR_TOKEN`, `SMELDR_MCP_URL` first, falling back to
  `FORGE_URL`, `FORGE_TOKEN`, `FORGE_MCP_URL` when unset. Both `.smeldr-cli.env`
  and `.forge-cli.env` are read (`.smeldr-cli.env` loaded first). `forge-cli init`
  now writes `.smeldr-cli.env` with `SMELDR_*` variable names. Closes T78.

---

## [0.10.0] — 2026-06-01

Block-system commands (T32 component 6, Amendment A119) + T77 table output.

> Note: the `0.9.0`–`0.9.3` entries are absent from this CHANGELOG, and the
> `cliVersion` const had lagged at `0.9.0` despite tags shipping through `0.9.3`
> (audit list, oauth revoke, etc.). This release resyncs the const to `0.10.0`;
> the missing 0.9.x entries are a pre-existing gap to backfill separately.

### Added

- `block` command group mirroring the 12 block MCP tools (T32 component 3):
  - `block node create|update|get|list|publish|archive` (Author role).
  - `block section add|reorder|remove` and `block item add|reorder|remove` (Editor role).
- `block node list` prints an aligned table (columns ID, type_name, status, slug);
  `--json` switches to raw JSON.
- Block `Fields` keys are case-sensitive PascalCase; `--field K=V` preserves casing
  (and `--fields <json>` passes an object through verbatim).

### Changed

- `cliVersion` resynced `0.9.0` → `0.10.0`.

## [0.8.0] — 2026-05-14

forge-social CLI parity — credential get/delete, platform configure, X support.

### Added

- `forge-cli social credential get <id>` — retrieves a single credential by slug via `get_social_credential`.
- `forge-cli social credential delete <id>` — permanently deletes a credential via `delete_social_credential`.
- `forge-cli social platform configure --platform mastodon|linkedin|x --client-id <id> --client-secret <secret> --redirect-url <url> [--instance-url <url>] [--success-url <url>]` — configures per-platform OAuth 2.0 app credentials via `create_platform_config`. Never echoes secrets.

### Changed

- `forge-cli social credential create` — now accepts `--platform x`. Fatal error if `--instance-url` is provided for platform `x`.
- `forge-cli social post create/queue` — help text updated to show `mastodon|linkedin|x` for `--platform`.

---

## [0.7.0] — 2026-05-12

forge-social CLI commands — post, credential, and schedule management (M18+M19).

### Added

- `forge-cli social post create --credential <id> --body "..." [--platform mastodon|linkedin] [--at <RFC3339>]` — creates a draft or scheduled post via MCP.
- `forge-cli social post queue --credential <id> --body "..." [--platform ...]` — enqueues a post for the next available PublicationSchedule slot (status `queued`).
- `forge-cli social post list [--status <status>]` — lists posts filtered by status.
- `forge-cli social post get <id>` — retrieves a single post.
- `forge-cli social post publish <id>` — publishes a post immediately.
- `forge-cli social post archive <id>` — archives a post.
- `forge-cli social post delete <id>` — permanently deletes a post.
- `forge-cli social credential create --platform mastodon|linkedin [--instance-url <url>]` — starts OAuth flow and prints the authorization URL.
- `forge-cli social credential list` — lists all configured credentials.
- `forge-cli social schedule create --credential <id> --slot "<weekday> HH:MM IANA/TZ" [--slot ...]` — creates a recurring publication schedule.
- `forge-cli social schedule show --credential <id>` — shows the schedule for a credential.
- `forge-cli social schedule pause --credential <id>` — suspends the schedule.
- `forge-cli social schedule resume --credential <id>` — reactivates a paused schedule.
- `forge-cli social schedule delete --credential <id>` — removes the schedule.

---

## [0.6.0] — 2026-05-09

Media subcommands and AVIF support (Milestone 13, Amendment A93).

### Added

- `forge-cli media upload <file> [--description <text>]` — uploads a file to
  the Forge media library via `POST /media` with the configured bearer token.
  `--description` is required for image files (WCAG 1.1.1). Prints the returned
  URL on success.
- `forge-cli media list [--type image|document|video|audio|other]` — lists all
  media records. Prints a table of ID, type, upload date, and URL.
- `forge-cli media delete <id>` — permanently deletes a media record by ID.
- `.avif` added to the image extension set — AVIF uploads now require
  `--description`, consistent with forge-media v1.2.0 AVIF support.

---

## [0.5.0] — 2026-05-08

Draft preview subcommand (Milestone 12, Amendment A92).

### Added

- `forge preview <prefix> <slug>` — generates a signed draft preview URL via the
  `create_preview_url` MCP tool and prints it to stdout. Requires Admin role.
  The URL grants read access to Draft or Scheduled content for the token lifetime
  (default 12 h). Archived items return 404 even with a valid token.

---

## [0.4.0] — 2026-05-08

Webhook management commands (Milestone 11 — CLI parity for forge-mcp webhook tools).

### Added

- `forge webhook create --url URL --events EVENT,...` — registers a new outbound
  webhook endpoint (HTTPS only). Prints the signing secret once.
- `forge webhook list` — lists all registered endpoints.
- `forge webhook delete <endpoint-id>` — removes an endpoint by ID.
- `forge webhook deliveries <job-id>` — shows delivery log for a job.
- `forge webhook retry <job-id>` — re-queues a dead job for delivery.

---

## [0.3.0] — 2026-05-04

### Added

- `forge-cli init [--url URL] [--bootstrap-token TOKEN] [--name NAME] [--days N] [--force]`
  Bootstrap a new Forge instance: validates reachability (`/_health`), creates
  a named admin token via the bootstrap token, writes `.forge-cli.env`
  (`FORGE_URL` + `FORGE_TOKEN`), and verifies the new token. Use `--force` to
  overwrite an existing env file.

---

## [0.2.1] — 2026-05-02

Patch release — no code changes. Re-tag to refresh module proxy cache after
vanity URL migration to `forge-cms.dev`.

---

## [0.2.0] — 2026-04-30

Go 1.26.2 and module path migration to `forge-cms.dev` (Amendment A76).

### Changed

- `go.mod`: module path renamed from `github.com/forge-cms/forge-cli` to
  `forge-cms.dev/forge-cli`; `go` directive bumped from `1.22` to `1.26.2`.

---

## [0.1.0] — 2026-04-07

Initial release — operator CLI for Forge instances (Decision 28).

### Added

- `forge-cli <type> create [--from file]` — create a Draft via `POST /{prefix}`
- `forge-cli <type> update <slug> [--from file]` — GET-then-PUT field overlay
- `forge-cli <type> publish <slug>` — GET-then-PUT with `Status: published`
- `forge-cli <type> unpublish <slug>` — GET-then-PUT with `Status: draft`
- `forge-cli <type> archive <slug>` — GET-then-PUT with `Status: archived`
- `forge-cli <type> delete <slug>` — `DELETE /{prefix}/{slug}`
- `forge-cli <type> list [--status <s>]` — list items with optional status filter
- `forge-cli <type> get <slug>` — print a single item as JSON
- `forge-cli token create --name <n> --role <r> [--ttl <d>]` — issue a token via MCP
- `forge-cli token list` — list tokens via MCP
- `forge-cli token revoke <id>` — revoke a token via MCP
- `forge-cli status` — `GET /_health`, print JSON
- Config via `FORGE_URL`, `FORGE_TOKEN`, `FORGE_MCP_URL` env vars or `.forge-cli.env`
- YAML-subset frontmatter parser (no external dependencies)
- Pure stdlib — zero third-party dependencies
