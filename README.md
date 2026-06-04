# forge-cli

Command-line interface for Forge CMS instances. Manage content and tokens
from a terminal or CI/CD pipeline.

Zero third-party dependencies — requires only Go 1.26 or later.

---

## Installation

```bash
go install forge-cms.dev/forge-cli@latest
```

Or build from source:

```bash
git clone https://github.com/forge-cms/forge
cd forge/forge-cli
go build -o forge-cli .
```

---

## Configuration

Set environment variables or create a `.forge-cli.env` file in your working
directory (values already set in the environment take precedence):

```env
FORGE_URL=https://mysite.com
FORGE_TOKEN=my-bearer-token
FORGE_MCP_URL=https://mysite.com/mcp/message
```

`FORGE_MCP_URL` defaults to `{FORGE_URL}/mcp/message` if not set. It is only
required for `token` commands.

---

## Content commands

All content commands take the URL path prefix of the content type as the first
argument (e.g. `posts`, `pages`).

### Create a draft

```bash
forge-cli posts create --from post.md
```

`--from` reads a YAML-subset frontmatter file. Omit `--from` to read from stdin.

Frontmatter format:

```
---
Title: My Post
Body: Hello world
Tags: [go, forge]
---
Optional body text appended to Body if Body is blank in the header.
```

### Update (field overlay)

```bash
forge-cli posts update my-post --from updated.md
```

GETs the existing item and overlays only the fields present in the file.
Fields absent from the file are preserved unchanged.

### Lifecycle transitions

```bash
forge-cli posts publish my-post
forge-cli posts unpublish my-post
forge-cli posts archive my-post
```

### Delete

```bash
forge-cli posts delete my-post
```

### List

```bash
forge-cli posts list
forge-cli posts list --status draft
forge-cli posts list --status published
```

### Get a single item

```bash
forge-cli posts get my-post
```

---

## Token commands

Token commands require `FORGE_MCP_URL` and an Admin-role token in `FORGE_TOKEN`.

### Create a token

```bash
forge-cli token create ci-deploy author 30
```

Arguments: `<name> <role> <ttl-days>`. Roles: `guest`, `author`, `editor`,
`admin`. TTL is an integer number of days (e.g. `30` for 30 days). Prints
the plaintext token once — copy it immediately.

### List tokens

```bash
forge-cli token list
```

### Revoke a token

```bash
forge-cli token revoke <id>
```

Revocation is permanent and takes effect immediately.

---

## Status check

```bash
forge-cli status
```

Calls `GET /_health` and prints the JSON response. Exits non-zero if the
server is unreachable.

---

## Social commands

Requires a running [forge-social](https://github.com/forge-cms/forge-social) v0.5.0+ instance wired to the Forge MCP server.

### Posts

```bash
forge-cli social post create --credential <id> --body "..." [--platform mastodon|linkedin|x] [--at <RFC3339>]
forge-cli social post queue  --credential <id> --body "..." [--platform mastodon|linkedin|x]
forge-cli social post list   [--status draft|queued|scheduled|published|failed|archived]
forge-cli social post get    <slug>
forge-cli social post publish <slug>
forge-cli social post archive <slug>
forge-cli social post delete  <slug>
```

`post create` without `--at` creates a draft. `--at` schedules for a specific time.  
`post queue` is shorthand for `post create` with `status: queued` — the post is published at the next available slot in the credential's `PublicationSchedule`.

### Credentials

```bash
forge-cli social credential create --platform mastodon|linkedin|x [--instance-url <url>]
forge-cli social credential list
forge-cli social credential get    <id>
forge-cli social credential delete <id>
```

`credential create` prints the OAuth authorisation URL. Open it in a browser to connect the account.  
`--instance-url` is only accepted for platform `mastodon`. Providing it for `x` is a fatal error.

### Platform configuration

Configures the OAuth 2.0 app credentials for a platform (client ID, client secret, redirect URL).  
Requires Admin role. Credentials are stored encrypted server-side and never echoed back.

```bash
forge-cli social platform configure \
  --platform mastodon|linkedin|x \
  --client-id <id> \
  --client-secret <secret> \
  --redirect-url <url> \
  [--instance-url <url>]   # mastodon only \
  [--success-url <url>]
```

### Schedules

```bash
forge-cli social schedule create --credential <id> --slot "<weekday> HH:MM IANA/TZ" [--slot ...]
forge-cli social schedule show   --credential <id>
forge-cli social schedule pause  --credential <id>
forge-cli social schedule resume --credential <id>
forge-cli social schedule delete --credential <id>
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
forge-cli block node create --type hero --field Headline="Welcome" --field Subtext="Intro"
forge-cli block node create --type content_block --fields '{"Title":"Hi","Body":"**bold**"}'
forge-cli block node update <id> --field Title="New title"
forge-cli block node get <id>
forge-cli block node list --type hero --status published      # aligned table
forge-cli block node list --json                              # raw JSON
forge-cli block node publish <id>
forge-cli block node archive <id>

# Composition (Editor role)
forge-cli block section add <page_id> <block_id>
forge-cli block section reorder <page_id> <id1,id2,id3>
forge-cli block section remove <page_id> <block_id>
forge-cli block item add <collection_id> <block_id>
forge-cli block item reorder <collection_id> <id1,id2>
forge-cli block item remove <collection_id> <block_id>
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
forge-cli nav list

# List as raw JSON
forge-cli nav list --json

# Create a navigation item
forge-cli nav create --label "Learn" --path /learn [--parent-id <id>] [--module <module>] [--hidden] [--ghost] [--sort-order <n>]

# Update a navigation item (absent flags preserve stored values)
forge-cli nav update <id> [--label <text>] [--path <path>] [--parent-id <id>] [--module <module>] [--hidden] [--ghost] [--sort-order <n>]

# Delete a navigation item and all its descendants
forge-cli nav delete <id>
```

`nav list` prints a table with columns: ID, LABEL, PATH, PARENT, HIDDEN, GHOST,
SORT. Use `--json` for the full raw response including the `module` field.

`nav create` requires `--label`. All other flags are optional. `--hidden` and
`--ghost` are boolean switches (no value). `--sort-order` takes an integer
(lower = earlier within the same parent level).

Writes (`create`, `update`, `delete`) require the server to be in DB nav mode;
`nav list` works in any nav mode. If writes are attempted against a non-DB
instance the server returns an error that forge-cli surfaces directly.

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md).
