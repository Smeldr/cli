# smeldr-cli

Command-line interface for Smeldr instances. Manage content and tokens
from a terminal or CI/CD pipeline.

Zero third-party dependencies — requires only Go 1.26 or later.

---

## Installation

```bash
go install smeldr.dev/cli/cmd/smeldr-cli@latest
```

Or build from source:

```bash
git clone https://github.com/Smeldr/cli
cd cli
go build -o smeldr-cli ./cmd/smeldr-cli
```

---

## Migrating from v0.13.x

The binary was renamed `forge-cli` → `smeldr-cli` in v0.14.0. Update any scripts
that invoke `forge-cli` to use `smeldr-cli`. The install path changed to
`go install smeldr.dev/cli/cmd/smeldr-cli@latest`.

Legacy `FORGE_*` env vars and `.forge-cli.env` are still read as fallbacks.

---

## Configuration

Set environment variables or create a `.smeldr-cli.env` file in your working
directory (values already set in the environment take precedence):

```env
SMELDR_URL=https://mysite.com
SMELDR_TOKEN=my-bearer-token
SMELDR_MCP_URL=https://mysite.com/mcp/message
```

`SMELDR_MCP_URL` defaults to `{SMELDR_URL}/mcp/message` if not set. It is only
required for MCP-based commands (token, block, nav, redirect, social, etc.).

Legacy `FORGE_URL` / `FORGE_TOKEN` / `FORGE_MCP_URL` env vars and `.forge-cli.env`
are still accepted as fallbacks (T86/T87 compatibility).

---

## Content commands

All content commands take the URL path prefix of the content type as the first
argument (e.g. `posts`, `pages`).

### Create a draft

```bash
smeldr-cli posts create --from post.md
```

`--from` reads a YAML-subset frontmatter file. Omit `--from` to read from stdin.

Frontmatter format:

```
---
Title: My Post
Body: Hello world
Tags: [go, smeldr]
---
Optional body text appended to Body if Body is blank in the header.
```

### Update (field overlay)

```bash
smeldr-cli posts update my-post --from updated.md
```

GETs the existing item and overlays only the fields present in the file.
Fields absent from the file are preserved unchanged.

### Lifecycle transitions

```bash
smeldr-cli posts publish my-post
smeldr-cli posts unpublish my-post
smeldr-cli posts archive my-post
```

### Delete

```bash
smeldr-cli posts delete my-post
```

### List

```bash
smeldr-cli posts list
smeldr-cli posts list --status draft
smeldr-cli posts list --status published
```

### Get a single item

```bash
smeldr-cli posts get my-post
```

---

## Token commands

Token commands require `SMELDR_MCP_URL` and an Admin-role token in `SMELDR_TOKEN`.

### Create a token

```bash
smeldr-cli token create ci-deploy author 30
```

Arguments: `<name> <role> <ttl-days>`. Roles: `guest`, `author`, `editor`,
`admin`. TTL is an integer number of days (e.g. `30` for 30 days). Prints
the plaintext token once — copy it immediately.

### List tokens

```bash
smeldr-cli token list
```

### Revoke a token

```bash
smeldr-cli token revoke <id>
```

Revocation is permanent and takes effect immediately.

---

## Status check

```bash
smeldr-cli status
```

Calls `GET /_health` and prints the JSON response. Exits non-zero if the
server is unreachable.

---

## Logs command

```bash
smeldr-cli logs [--level LEVEL] [--limit N] [--since RFC3339] [--json]
```

Calls `GET /_logs` directly (not via MCP) and prints a table of captured log
entries. Requires Admin role. The server must call `app.CaptureLogs()` (core
v1.36.0+).

Flags:

- `--level <LEVEL>` — minimum level inclusive (e.g. `warn`, `error`)
- `--limit <N>` — most recent N entries
- `--since <RFC3339>` — entries strictly after this timestamp
- `--json` — print raw JSON envelope `{capacity, count, dropped, entries}`

Default output is a table with columns: TIMESTAMP, LEVEL, SEQ, MESSAGE.
Entries are newest-first. A footer is printed when the ring buffer overflowed
and entries were dropped.

Since `/_logs` does not go through MCP, it works even when the MCP server is
the component that is failing.

---

## Social commands

Requires a running [smeldr.dev/social](https://smeldr.dev/docs/social) v0.8.0+ instance wired to the Smeldr MCP server.

### Posts

```bash
smeldr-cli social post create --credential <id> --body "..." [--platform mastodon|linkedin|x] [--at <RFC3339>]
smeldr-cli social post queue  --credential <id> --body "..." [--platform mastodon|linkedin|x]
smeldr-cli social post list   [--status draft|queued|scheduled|published|failed|archived]
smeldr-cli social post get    <slug>
smeldr-cli social post publish <slug>
smeldr-cli social post archive <slug>
smeldr-cli social post delete  <slug>
```

`post create` without `--at` creates a draft. `--at` schedules for a specific time.  
`post queue` is shorthand for `post create` with `status: queued` — the post is published at the next available slot in the credential's `PublicationSchedule`.

### Credentials

```bash
smeldr-cli social credential create --platform mastodon|linkedin|x [--instance-url <url>]
smeldr-cli social credential list
smeldr-cli social credential get    <id>
smeldr-cli social credential delete <id>
```

`credential create` prints the OAuth authorisation URL. Open it in a browser to connect the account.  
`--instance-url` is only accepted for platform `mastodon`. Providing it for `x` is a fatal error.

### Platform configuration

Configures the OAuth 2.0 app credentials for a platform (client ID, client secret, redirect URL).  
Requires Admin role. Credentials are stored encrypted server-side and never echoed back.

```bash
smeldr-cli social platform configure \
  --platform mastodon|linkedin|x \
  --client-id <id> \
  --client-secret <secret> \
  --redirect-url <url> \
  [--instance-url <url>]   # mastodon only \
  [--success-url <url>]
```

### Schedules

```bash
smeldr-cli social schedule create --credential <id> --slot "<weekday> HH:MM IANA/TZ" [--slot ...]
smeldr-cli social schedule show   --credential <id>
smeldr-cli social schedule pause  --credential <id>
smeldr-cli social schedule resume --credential <id>
smeldr-cli social schedule delete --credential <id>
```

Slot format: `"<weekday> <HH:MM> <IANA timezone>"` — e.g. `"monday 09:00 Europe/Copenhagen"`.  
Multiple `--slot` flags define multiple firing times per week.  
Each credential may have at most one schedule.

---

## Block commands

The `block` group manages the block system (T32) over MCP, mirroring the block MCP
tools. Blocks are addressed by **ID** (no slug).

```bash
# Nodes (Author role)
smeldr-cli block node create --type hero --field Headline="Welcome" --field Subtext="Intro"
smeldr-cli block node create --type content_block --fields '{"Title":"Hi","Body":"**bold**"}'
smeldr-cli block node update <id> --field Title="New title"
smeldr-cli block node get <id>
smeldr-cli block node list --type hero --status published      # aligned table
smeldr-cli block node list --json                              # raw JSON
smeldr-cli block node publish <id>
smeldr-cli block node archive <id>

# Composition (Editor role)
smeldr-cli block section add <page_id> <block_id>
smeldr-cli block section reorder <page_id> <id1,id2,id3>
smeldr-cli block section remove <page_id> <block_id>
smeldr-cli block item add <collection_id> <block_id>
smeldr-cli block item reorder <collection_id> <id1,id2>
smeldr-cli block item remove <collection_id> <block_id>
```

**Field keys are case-sensitive — use PascalCase** (`Title`, `Body`, `Headline`) to
match the block templates. `--field K=V` preserves the key's case exactly; use
`--fields '<json>'` for typed or nested values (e.g. a `Link` object). `block node
list` prints a table by default; add `--json` for raw output.

---

## Nav commands

The `nav` group manages the navigation tree over MCP. Requires Editor role and
a server configured with DB nav mode (`app.Nav(...)`).

```bash
# List navigation items (aligned table)
smeldr-cli nav list

# List as raw JSON
smeldr-cli nav list --json

# Create a navigation item
smeldr-cli nav create --label "Learn" --path /learn [--parent-id <id>] [--module <module>] [--hidden] [--ghost] [--sort-order <n>]

# Update a navigation item (absent flags preserve stored values)
smeldr-cli nav update <id> [--label <text>] [--path <path>] [--parent-id <id>] [--module <module>] [--hidden] [--ghost] [--sort-order <n>]

# Delete a navigation item and all its descendants
smeldr-cli nav delete <id>
```

`nav list` prints a table with columns: ID, LABEL, PATH, PARENT, HIDDEN, GHOST,
SORT. Use `--json` for the full raw response including the `module` field.

`nav create` requires `--label`. All other flags are optional. `--hidden` and
`--ghost` are boolean switches (no value). `--sort-order` takes an integer
(lower = earlier within the same parent level).

Writes (`create`, `update`, `delete`) require the server to be in DB nav mode;
`nav list` works in any nav mode. If writes are attempted against a non-DB
instance the server returns an error that smeldr-cli surfaces directly.

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md).
