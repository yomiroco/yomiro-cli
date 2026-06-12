---
name: yomiro-cli
description: >-
  Use the Yomiro CLI (`yomiro`) to authenticate operators, run the customer-side
  gateway daemon that bridges a local network/database to the Yomiro platform, and
  manage platform resources (dashboards, devices, captures, incidents). Use this
  skill whenever the user is working with the `yomiro` command — logging in,
  minting API tokens (`yomiro auth ...`), setting up or operating a gateway
  (`yomiro gw ...`, connecting a local Postgres through it), or scripting platform
  operations like migrating a dashboard between environments — even if they don't
  name the skill explicitly. Also use it when a `yomiro` command errors (403s,
  "not in the gateway allowlist", audience mismatches) and the user needs the
  correct flow.
---

# Yomiro CLI

`yomiro` is a single Go binary that does two jobs:

1. **Gateway daemon** (`yomiro gw ...`) — runs on a customer's machine and bridges
   their local network and database to the Yomiro platform over an outbound
   WebSocket tunnel. This is the headline use case.
2. **Platform control** (`yomiro auth|dashboard|device|capture|incident ...`) —
   authenticates an operator and manages tenant resources via the platform API.

Everything is scoped to one **tenant** and one **environment** (prod/dev/staging),
determined by who you logged in as and which API URL you target.

## Install / build

```bash
brew install yomiroco/yomiro/yomiro                 # macOS / Linux (Homebrew)
# or, without a package manager:
curl -fsSL https://raw.githubusercontent.com/yomiroco/yomiro-cli/main/install.sh | sh
yomiro --help
```

Other methods (Docker, pre-built binaries, `go install`, source) are in
[`INSTALL.md`](../../../INSTALL.md). To run a locally-built binary instead of the
installed one, build it and invoke it by path (`./bin/yomiro`) or symlink it onto
your PATH.

## Targeting an environment

Every command resolves the API URL and bearer token with this precedence
(**highest first**):

1. `--api-url` / `--token` flags
2. `YOMIRO_API_URL` / `YOMIRO_API_TOKEN` env vars
3. stored credentials (from `yomiro auth login`)
4. compiled-in defaults (prod: `https://api.yomiro.io`)

Known environments: prod `https://api.yomiro.io`, dev `https://api.dev.yomiro.io`,
staging `https://api.staging.yomiro.io`. Credentials are **per environment** — a
token minted against dev will not work against prod. To act on two environments in
one shell (e.g. migrating between them), set `YOMIRO_API_URL`/`YOMIRO_API_TOKEN`
inline per command rather than relying on the single stored login.

Output format is controlled by `--output json|yaml|table` (default `json`), so
platform commands pipe cleanly into `jq`.

## Authentication

```bash
yomiro auth login                 # device-code flow → mints + stores a 30-day API key
yomiro auth whoami                # show signed-in user, tenant, token prefix
yomiro auth login --web           # browser scope-picker: choose scopes + lifetime interactively
yomiro auth logout                # revoke the local token and clear stored credentials
```

`login` defaults to a read-only scope set. It mints a long-lived API key (a
`yom_pat_*` token) and saves it as your credential — it does **not** keep your Auth0
session around. Override the scopes with `--scopes`, or use `--web` to pick them in
the browser.

Target a non-default environment by setting the API URL and the matching Auth0
client/audience for that tenant, e.g. dev:

```bash
YOMIRO_API_URL=https://api.dev.yomiro.io \
YOMIRO_AUTH0_AUDIENCE=https://api.dev.yomiro.io \
  yomiro auth login
```

### Minting an API token (`auth token create`)

`auth token create` mints a **scoped** key — for a gateway, for CI, for a script.
Because the platform forbids one long-lived key from minting another, this command
acquires a **fresh** interactive credential (it never reuses your stored login
token) and **prints** the new key once without saving it:

```bash
yomiro auth token create --name dev-gw --scopes gateway:tunnel
#   → runs a device-code login, then prints:  yom_pat_...
```

Three ways to authorize the mint:

- **Device-code (default):** opens the browser sign-in, then mints.
- **`--web`:** pick scopes/lifetime in the browser; `--name`/`--scopes`/`--ttl` only
  pre-seed the picker.
- **Non-interactive:** set `YOMIRO_API_TOKEN` (or `--token`) to an Auth0 **JWT** to
  skip the browser — useful in CI. (Get a JWT to replay with
  `yomiro auth login --debug-jwt`, which prints/writes the token without minting.)

`--ttl` takes a Go duration (`720h` = 30 days) or `0` for never. List and revoke
keys with `yomiro auth token list` / `yomiro auth token revoke <key-id>`.

> The minted token is shown **once**. Copy it immediately.

## Gateway: bridge a local database to the platform

The gateway runs on a machine inside the customer network, dials out to the platform
(no inbound ports), and exposes a **read-only, allowlisted** view of a local
Postgres database (plus other connectors). The platform — agents, dashboards,
data-sources — then queries that database *through* the gateway tunnel. The operator
never runs the queries; they configure what the gateway is allowed to expose.

### One-command bootstrap

```bash
yomiro gw init --from-login \
  --db-url 'postgres://readonly_user:pass@localhost:5432/appdb' \
  --endpoint wss://api.dev.yomiro.io/api/v1/gateway/ws \
  --allow-table public.orders --allow-table public.customers \
  --block-column public.customers.email
yomiro gw up      # install + start as a system service
```

`--from-login` mints the `gateway:tunnel` token for you (device-code, or add `--web`)
and stores it in the OS keyring — no separate `auth token create` step. Alternatively
pass a pre-minted token with `--token <yom_pat_...>` (e.g. from the web app's
"Connect a gateway" button or CI); `--token` and `--from-login` are mutually
exclusive and exactly one is required.

`gw init` writes `gw.yaml` (path printed on success, typically
`~/.config/yomiro/gw.yaml`) and stashes the token in the keyring. `gw up` registers
and starts the daemon as a system service; `gw run` instead runs it in the
foreground (no service install) — best for local testing.

### Connecting a local database through an already-running gateway

If the daemon is already up (`yomiro gw up`) and you want to point it at a local
database — or change which tables it exposes — you do **not** re-run `init` or
restart the service. Edit the database config and hot-reload:

1. Edit `~/.config/yomiro/gw.yaml`, `database` section:

   ```yaml
   database:
     url: postgres://readonly_user:pass@localhost:5432/appdb
     read_only: true            # only SELECT is allowed through the proxy
     allowed_tables:            # FQNs — REQUIRED, see gotcha below
       - public.orders
       - public.customers
     blocked_columns:
       - public.customers.email
     max_rows_per_query: 10000
     max_connections: 5
     query_timeout_seconds: 30
   ```

2. Apply it without downtime:

   ```bash
   yomiro gw reload     # re-reads gw.yaml and reconnects the DB pool
   yomiro gw status     # confirm the daemon is connected and the postgres connector is up
   ```

The platform side can now query the local DB through the tunnel. To verify
end-to-end, run a query from the platform (a dashboard widget or agent backed by the
gateway data-source) and watch `yomiro gw logs` (add `--audit` to see the audited SQL
the gateway received).

> **Critical allowlist gotcha:** the gateway's SQL proxy rejects **any** table not in
> `allowed_tables`. An empty allowlist therefore blocks *every* query — a
> freshly-init'd gateway with no `--allow-table`/`allowed_tables` will refuse all
> traffic with `table "…" is not in the gateway allowlist`. Always list the exact
> `schema.table` FQNs you intend to expose. Combined with `read_only: true`
> (the default), only `SELECT` statements against allowlisted tables, minus any
> `blocked_columns`, reach the database. This is the security boundary — keep it
> tight.

### Operating the daemon

```bash
yomiro gw status            # daemon state + connector health (talks to the local control socket)
yomiro gw logs              # tail the daemon log   (--audit for the audited-SQL log)
yomiro gw reload            # re-read gw.yaml and reconnect (after editing config)
yomiro gw pause             # pause tool/query dispatch without disconnecting
yomiro gw resume            # resume dispatch
yomiro gw down              # stop the service and unregister the gateway
```

## Platform resources

Resource groups: `dashboard`, `device`, `capture`, `incident`, `ai-config`,
`location`, `device-group`, `data-source`, `user`, `agent`, `team`, `alert`,
`ai-provider`, `inspection-profile`, `model`, `ref-sheet`, `otel-endpoint`,
`analytics`, `organization`, `entity-history` (plus a `skill` group). Each has
`list`/`get`/etc. (run `yomiro <group> --help` for the verbs a group exposes).
Write-style commands that take a request body use
a consistent pattern: `--skeleton` prints a starter JSON template, and
`--json-body` accepts the body as a literal string or `@file.json`.

```bash
yomiro device list --output table
yomiro device create --skeleton                       # print the request-body template
yomiro device create --json-body @new-device.json     # create from an edited template
yomiro device update <deviceId> --json-body '{"status":"maintenance"}'
yomiro capture list
yomiro capture get <captureId>
yomiro capture get-detections <captureId>             # detections for a capture
yomiro incident list
```

### Migrating a dashboard between environments

Dashboards round-trip via `export` → `import`. Because credentials are per
environment, pass each environment's token inline. First mint a token in **each**
tenant with dashboard scopes:

```bash
# one-time: mint a token per environment (see "Minting an API token")
DEV_TOKEN=$(YOMIRO_API_URL=https://api.dev.yomiro.io yomiro auth token create \
  --name dash-migrate --scopes dashboards:read | grep yom_pat_ | tr -d ' ')
STG_TOKEN=...   # same against https://api.staging.yomiro.io, with dashboards:read,dashboards:write
```

Then export from the source environment and import into the target:

```bash
# Export from dev
YOMIRO_API_URL=https://api.dev.yomiro.io YOMIRO_API_TOKEN=$DEV_TOKEN \
  yomiro dashboard export <dashboardId> > dashboard.json

# Import into staging
YOMIRO_API_URL=https://api.staging.yomiro.io YOMIRO_API_TOKEN=$STG_TOKEN \
  yomiro dashboard import --json-body @dashboard.json
```

`dashboard export` emits the portable dashboard definition; `dashboard import`
recreates it in the target tenant. (`dashboard get`/`create`/`update` are the
lower-level CRUD equivalents if you need to transform the payload in between — e.g.
remap data-source IDs that differ across environments. Inspect the exported JSON
before importing; environment-specific references like data-source or device IDs may
need rewriting for the target tenant.)

### Adding a capture rule

Capture **rules** — what triggers a capture and how detection runs — are part of the
**AI config**, which is attached to a **device group** (`ai-config` keyed by
`<deviceGroupId>`). The rule itself lives in the free-form `capture_config` object;
other AI-config fields (`enabled`, `text_queries`, `model_pipeline`, `threshold`, …)
control detection. The `capture` group is review-only (`list`/`get`/`get-detections`);
authoring happens via `ai-config`.

```bash
# 1. See the full request-body template (lists every AI-config field, incl. capture_config)
yomiro ai-config create-or-replace <deviceGroupId> --skeleton > ai-config.json

# 2. Edit ai-config.json — set the capture rule + detection settings — then apply.
#    create-or-replace sets the whole config; update patches an existing one.
yomiro ai-config update <deviceGroupId> --json-body '{
  "enabled": true,
  "text_queries": ["forklift", "person without helmet"],
  "capture_config": { ... }
}'
```

The inner shape of `capture_config` is defined platform-side (it's a free-form object
in the API). Use `--skeleton` and the web app's capture-rule editor as the source of
truth for its keys rather than guessing. `ai-config get <deviceGroupId>` shows the
current config for a device group.

## Onboarding a new plant

Setting up a brand-new plant/tenant is a multi-step journey — location hierarchy
(plant → line → unit), users, device groups, data sources, devices & cameras,
capture rules, the gateway, and the first dashboards. The CLI has dedicated groups
for each: `location`, `user`, `device-group`, `data-source`, `device`, `ai-config`,
`gw`, `dashboard`.

For the full step-by-step walkthrough — including creating `gw.yaml` before
`gw up`, onboarding users via Auth0, and wiring data sources through the gateway —
read [`references/onboarding.md`](references/onboarding.md).

## Troubleshooting

- **`403 ... Could not validate credentials` on `auth token create`** — your stored
  token expired or points at another environment. `token create` acquires a fresh
  credential itself, so just re-run it (it will device-code login). If scripting,
  pass a current Auth0 JWT via `YOMIRO_API_TOKEN`.
- **`403 ... Organization membership required` / `Invalid audience`** — the token's
  Auth0 audience doesn't match the backend you're hitting. Ensure `YOMIRO_API_URL`
  and `YOMIRO_AUTH0_AUDIENCE` describe the same environment when you log in.
- **`table "…" is not in the gateway allowlist`** — add the `schema.table` to
  `allowed_tables` in `gw.yaml` and `yomiro gw reload`. See the allowlist gotcha.
- **`read-only: only SELECT statements are allowed`** — the gateway DB is read-only
  by default; non-SELECT SQL is rejected by design.
- **`yomiro gw status` can't reach the daemon** — the service isn't running; start it
  with `yomiro gw up` (or `yomiro gw run` in the foreground to see startup errors).
- **`gw status` shows `"connected": false`; logs show `WebSocket dial … got 404`** —
  the daemon is dialing a `platform.endpoint` where the gateway WebSocket route
  isn't served (a 404 on the WS handshake means "route absent", not an auth issue).
  `gw init` defaults the endpoint to **prod** (`wss://api.yomiro.io/...`); point it at
  the environment where the gateway service actually runs and where your token is
  valid (`--endpoint`, or edit `platform.endpoint` in `gw.yaml`), then restart the
  daemon. An auth problem would instead surface as a `4003`/close-code or 401/403,
  not a 404.
- **Edited `gw.yaml` but nothing changed** — run `yomiro gw reload`; it re-reads
  `gw.yaml` and reconnects (rebuilding the DB pool only if the URL changed, and the
  live connection only after the new config validates). Endpoint, allowlist,
  token-ref, and connector edits all apply without a restart. If `reload` reports an
  error the old config stays live — fix `gw.yaml` and reload again. Confirm with
  `yomiro gw status`.
- **`yomiro gw logs` is empty** — when run as a service the daemon's output is
  captured by the service manager, not the file `gw logs` tails. On macOS (launchd)
  look at `~/io.yomiro.gw.out.log` / `~/io.yomiro.gw.err.log`.
