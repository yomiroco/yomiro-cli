# Yomiro CLI — Architecture & Contributor Guide

This document explains how the `yomiro` Go CLI (`cli/`) is built so you can extend
it without reverse-engineering it. It is the depth layer behind two skills:

- **`cli-dev`** (repo-root `.claude/skills/cli-dev/`) — the internal dev skill that
  points here. (Dev-only; kept out of the public mirror — see [The public mirror](#the-public-mirror).)
- **`yomiro-cli`** (`cli/.claude/skills/yomiro-cli/`) — the user-facing "how do I
  *use* the CLI" skill.

For **running tests / building / live login flows**, see the `cli-testing` skill
(`.claude/skills/cli-testing/SKILL.md`) — this doc cross-links it rather than
duplicating it.

---

## 1. Big picture

`yomiro` is one Go binary (Cobra) with two jobs:

1. **Gateway daemon** (`yomiro gw …`) — runs on a customer machine, bridges their
   LAN/Postgres to the platform over an outbound WebSocket tunnel.
2. **Platform control** (`yomiro auth|dashboard|device|capture|…`) — authenticates
   an operator and drives the platform REST API.

The command tree is assembled in [`cmd/yomiro/root.go`](cmd/yomiro/root.go):

```
newRootCmd()
  ├── version                       (cmd/yomiro/*.go)
  ├── auth   …                      (internal/auth, hand-written)
  ├── gw     …                      (internal/gw, hand-written)
  └── <platform groups> + skill     (internal/platform.AddTo → generated + overrides)
```

`root.go` declares the four **persistent flags** every subcommand inherits:
`--env`, `--api-url`, `--token`, `--output` (default `json`). Default env is `prod`.

Two layers of platform commands:

- **Generated** from the backend OpenAPI spec (`internal/platform/generated/*.gen.go`)
  — one `New<Group>Cmd` constructor per OpenAPI tag.
- **Hand-written overrides** (`internal/platform/overrides/`) — take precedence over
  a generated subcommand of the same name, or add commands a generated group lacks.

Only a small **allowlist** of generated groups is actually exposed (see
[Command wiring](#3-command-wiring)).

---

## 2. Codegen pipeline (`make cli-codegen`)

Regenerate when the **backend API changes**. The target lives in the root
[`Makefile`](../Makefile) (`cli-codegen`) and runs four steps:

1. **Export the spec** — `cd backend && PYTHONPATH=. uv run python -m
   app.scripts.export_openapi > openapi.json`. Produces `backend/openapi.json`.

2. **Generate the typed client** — `cd cli && oapi-codegen --config oapi.yaml
   --exclude-operation-ids=images-reprocess_camera_image ../backend/openapi.json`.
   Writes `internal/platform/client/client.gen.go` (models + a
   `ClientWithResponses` with one `<OpID>WithResponse` method per operation). Config
   is [`cli/oapi.yaml`](oapi.yaml).

   > **The `--exclude-operation-ids` quirk:** `images-reprocess_camera_image` has a
   > malformed spec (path declares `{id}` but the handler binds `image_id` as a query
   > param), which oapi-codegen rejects. It is excluded at the CLI invocation, not in
   > `oapi.yaml`. See the note in `oapi.yaml`. If a *new* operation breaks codegen,
   > either fix the route or add it to this exclude list.

3. **Generate the command trees** — `cd cli && go run ./cmd/gen-platform-cmds
   ../backend/openapi.json ./internal/platform/generated`. See below.

4. **fmt + vet** — `go fmt ./internal/platform/...` then `go vet ./internal/platform/...`.

### What `gen-platform-cmds` does

A small standalone tool in [`cmd/gen-platform-cmds/`](cmd/gen-platform-cmds/):

- [`main.go`](cmd/gen-platform-cmds/main.go) — entry point. `Walk(openapi.json)`
  groups operations by tag, then `LoadClientMethods` parses
  `internal/platform/client/client.gen.go`, then `EmitGroup` writes one
  `<tag>.gen.go` per tag into the output dir.
- [`walker.go`](cmd/gen-platform-cmds/walker.go) — walks the OpenAPI doc
  (via `libopenapi`) into `map[tag][]Operation`, capturing method/path/params/body
  fields. **Primary tag wins** (`op.Tags[0]`); untagged operations are skipped.
- [`client_inspect.go`](cmd/gen-platform-cmds/client_inspect.go) — **the client's
  Go AST is the source of truth.** It scans `client.gen.go` for
  `<OpID>WithResponse` methods on `*ClientWithResponses`, extracting positional
  path args, `*…Params`, and typed-body presence. The spec is the *menu*; if
  oapi-codegen skipped an operation (multipart, unsupported schema), the generator
  emits a subcommand stub that errors at runtime instead of failing the build.
- [`emit.go`](cmd/gen-platform-cmds/emit.go) — renders the per-group
  `New<GoTag>Cmd(getClient func() *client.ClientWithResponses) *cobra.Command`
  from a `text/template`. It also:
  - **assigns subcommand verbs** (`assignVerbs`): derives a verb from the
    operationId by stripping the `<tag>-` prefix and the resource noun, falling
    back to HTTP-method defaults (`list`/`get`/`create`/`update`/`delete`) and
    panicking on an unresolvable collision so the spec author notices.
  - emits `--json-body` (literal or `@file`) for body-bearing ops and a
    `--skeleton` flag that prints a JSON template, and a `--<query-param>` flag
    per query param.

The constructor takes a `getClient` **factory** (not a client) so the persistent
`--api-url`/`--token`/`--env` flags can rebuild the client per invocation — see
[Credential precedence](#4-credential-precedence).

### Detecting drift

`make spec-parity` (root Makefile, `ENV=dev|prod`) fetches the *live* backend spec
and diffs it against the committed `backend/openapi.json`. A non-empty diff means
the committed spec — and therefore the generated CLI — is stale; re-run
`make cli-codegen`.

---

## 3. Command wiring

Generated groups + overrides are wired into root by
[`internal/platform/wire.go`](internal/platform/wire.go), called from
`root.go` via `platform.AddTo(cmd)`.

### The `publicGroups` allowlist (POLICY)

The generator emits ~51 group constructors, but **only ~20 are operator-facing**.
The rest are internal/test/web-only surfaces. The allowlist is the single
`publicGroups []groupSpec` slice in `wire.go`. Each entry is a `groupSpec`:

```go
type groupSpec struct {
    new func(func() *client.ClientWithResponses) *cobra.Command // generated constructor
    use string                                                  // singular-noun Use override ("" keeps the generated plural)
    why string                                                  // one-line rationale: why this group is operator-facing
}
```

Current entries (verify against `wire.go` — this is the live policy):
`dashboard`, `capture`, `incident`, `device`, `ai-config`, `location`,
`device-group`, `data-source`, `user`, `agent`, `team`, `alert`, `ai-provider`,
`inspection-profile`, `model`, `ref-sheet`, `otel-endpoint`, `analytics`,
`organization`, `entity-history`.

Two tests enforce the policy:

- `TestAddTo_WiresIntendedGroups` (`wire_test.go`) — asserts the **exact** wired set
  (the 20 above + the `skill` stub) and nothing else.
- `TestPublicGroupsCount` — asserts `len(publicGroups) == 20`.

Adding or removing a group is therefore a deliberate, test-gated edit.

### The singular-noun rename convention

Generated group names are the OpenAPI tag (plural, e.g. `dashboards`). The CLI
exposes **singular** nouns (`dashboard`, `device`, …) via the `use` field on each
`groupSpec`; `AddTo` overrides `cmd.Use` with it. Leave `use: ""` to keep the
generated plural.

### The overrides registry

[`internal/platform/overrides/registry.go`](internal/platform/overrides/registry.go)
is a tiny global store keyed by `"group/cmdName"`:

- Each override file calls `overrides.Register(group, cmd)` from an `init()`.
- `AllInGroup(group)` returns a group's overrides, sorted by name.
- In `AddTo`, after wiring each group, the loop calls `AllInGroup`; if an override's
  name collides with an existing (generated) subcommand it **removes the generated
  one first**, then adds the override → overrides win.

Examples: [`dashboard_render.go`](internal/platform/overrides/dashboard_render.go)
adds `dashboard render` (no generated equivalent);
[`skill_install.go`](internal/platform/overrides/skill_install.go) adds
`skill install`.

### The `skill` stub

`skill` has **no generated constructor** — there is no platform API for it (the
daemon manages local skill state). `AddTo` creates a bare `skill` cobra command so
the override registry has a home for `skill install`. Its name is added to the
override-pass group list alongside the allowlist so the two can't drift.

---

## 4. Credential precedence

All credential resolution funnels through one function:
[`credentials.Resolve(flagAPIURL, flagToken, profileAPIURL string) (apiURL, token string)`](internal/credentials/resolve.go).
Cobra is deliberately kept out of its signature so it is trivially table-testable.

**API-URL precedence, highest → lowest:**

1. `--api-url` flag (explicit, only the *changed* value is passed in)
2. `YOMIRO_API_URL` env var
3. the active **`--env` profile** API URL (only when `--env` was *explicitly*
   selected — see below)
4. stored credentials (`yomiro auth login`)
5. `credentials.DefaultAPIURL` = `https://api.yomiro.io`

**Token precedence:** `--token` > `YOMIRO_API_TOKEN` > stored credentials.
(Profiles carry no token.)

> This **unified** path replaced an earlier split where `auth token` commands read
> only the stored credential and ignored env/flags. Now `token list/revoke`
> ([`auth/token.go`](internal/auth/token.go)) and the platform groups
> ([`wire.go`](internal/platform/wire.go) `resolveCredentials`) all call
> `credentials.Resolve`.

**How the platform client picks this up:** `AddTo` installs a `PersistentPreRunE`
that calls `resolveCredentials(cmd)` and *rebuilds* the client on every invocation,
then the generated commands call `getClient()` at request time. This is the fix for
the old bug where `--api-url`/`--token` were declared on root but silently ignored
because the client was frozen at tree-construction time
(`TestAddTo_FlagOverridesAPIURL`, `TestAddTo_EnvOverridesAPIURL`,
`TestAddTo_FlagBeatsEnv`).

`changedFlag(cmd, name)` returns a flag's value only if `f.Changed` — that's the
"explicit flag" input `Resolve` expects (`""` means "unset").

---

## 5. Environment profiles (`--env`)

[`internal/envprofile/envprofile.go`](internal/envprofile/envprofile.go) maps a
single `--env {local|dev|staging|prod}` (or `YOMIRO_ENV`) to a full `Profile`:

| field | local | dev | staging | prod (default) |
|-------|-------|-----|---------|------|
| `APIURL` | `http://localhost:8000` | `https://api.dev.yomiro.io` | `https://api.staging.yomiro.io` | `https://api.yomiro.io` |
| `Auth0Domain` | `yomiro.eu.auth0.com` | same | same | same |
| `Auth0ClientID` | dev client | dev client | staging client | prod client |
| `Audience` | `https://api.dev.yomiro.io` | `https://api.dev.yomiro.io` | `https://api.staging.yomiro.io` | `https://api.yomiro.io` |
| `WSEndpoint` | `ws://localhost:8000/api/v1/gateway/ws` | `wss://api.dev.yomiro.io/api/v1/gateway/ws` | `wss://api.staging.yomiro.io/api/v1/gateway/ws` | `wss://api.yomiro.io/api/v1/gateway/ws` |
| `FrontendURL` | `http://localhost:5173` | `""` (derived → `dev.yomiro.io`) | `""` (derived → `staging.yomiro.io`) | `""` (derived → `app.yomiro.io`) |

> **`staging` Auth0 client ID is a placeholder** (`stagingClientID =
> "REPLACE_ME_..."`) until the real staging Auth0 application ID is filled in.
> `--env staging` API/WS/audience routing works today; interactive login under
> `--env staging` won't succeed until that constant is set.

Notes (verify in the source):
- **`local` deliberately reuses the dev Auth0 client + dev audience** — the
  documented working local setup. It requires the local backend to run with
  `AUTH0_API_IDENTIFIER=https://api.dev.yomiro.io`. (This audience trap is the #1
  local-login gotcha — the `cli-testing` skill covers it in depth.)
- `FrontendURL` is only spelled out for `local`; prod/dev derive it from the API
  host (`frontendFromAPI` in [`auth/login.go`](internal/auth/login.go)).
- `envprofile` is leaf-level on purpose: it imports only stdlib, cobra, and
  `internal/credentials` (for `DefaultAPIURL`). It must **not** import `auth`,
  `platform`, or `gw` (those depend on it; the reverse is an import cycle).

### `Active(cmd) (Profile, explicit, error)` and the "explicit" semantics

`Active` resolves the profile: changed `--env` flag → `YOMIRO_ENV` → `prod`
default. It returns an `explicit bool`: true if the operator *actively* selected
an env (changed flag or set `YOMIRO_ENV`), false for the implicit `prod` default.
An unknown name returns an error.

**Why `explicit` matters:** callers feed the profile's `APIURL` into
`credentials.Resolve` *only when `explicit`*:

```go
profileAPIURL := ""
if explicit {
    profileAPIURL = prof.APIURL
}
apiURL, token := credentials.Resolve(changedFlag(cmd, "api-url"), changedFlag(cmd, "token"), profileAPIURL)
```

- An **explicit** `--env dev` *should* beat a stored prod login (e.g. running
  `--env dev auth login` against a fresh tenant). So its API URL sits **above**
  stored creds in the precedence list.
- The **implicit** prod default must **not** override a stored login — otherwise
  the compiled prod URL would clobber whatever you logged into. So when implicit,
  the profile API URL is withheld and stored creds win.
- **Non-API-URL fields** (`Auth0*`, `WSEndpoint`, `FrontendURL`) are used
  *regardless* of `explicit`, so implicit prod still yields prod auth0 defaults
  and the prod WS endpoint.

### Who consumes profiles

- **`ResolveAuthConfig(cmd)`** ([`auth/acquire.go`](internal/auth/acquire.go)) —
  used by `auth login`, `auth token create`, and `gw init --from-login`. Returns
  `AuthConfig{APIURL, DC (device-code client w/ domain/client-id/audience), Profile}`.
  Each Auth0 field uses `flagEnvOr`: changed flag → `YOMIRO_AUTH0_*` env → profile.
- **`gw init`** ([`gw/init.go`](internal/gw/init.go)) — derives the gateway
  **WS endpoint** from `prof.WSEndpoint` (`resolveEndpoint`: an explicit
  `--endpoint` flag wins, else the profile's WSEndpoint). `gw init` uses
  `WSEndpoint` regardless of `explicit` (it's populated for every env, including
  implicit prod), so it discards the `explicit` bool.

---

## 6. The gateway daemon (`gw`)

Command tree in [`gw/cmd.go`](internal/gw/cmd.go): `init`, `run`, `up`, `down`,
`status`, `logs`, `pause`, `resume`, `reload`.

### `gw init` → `gw.yaml` + keyring

[`init.go`](internal/gw/init.go) bootstraps the gateway:
- Mints/accepts a `gateway:tunnel` token (`--token`, or `--from-login` to mint one
  interactively / via `YOMIRO_API_TOKEN` JWT, or `--web` browser picker), stores it
  in the OS keyring (`io.yomiro.gw/<gateway-id>`), and writes `gw.yaml` referencing
  it as `token_ref: keyring:io.yomiro.gw/<gateway-id>`.
- Resolves the platform **WS endpoint** from `--endpoint` or the `--env` profile.
- **Endpoint probe (added this session):** after writing config, `probeWSEndpoint`
  attempts a WS upgrade (no auth) to catch a wrong/undeployed URL. A 101 upgrade or
  a 401/403 means the route exists (auth happens at run time) → "reachable"; a 404
  or unreachable host → an actionable warning. **Non-fatal** — config is already
  written.

### `gw run` → the reconnect loop

[`run.go`](internal/gw/run.go) loads `gw.yaml`, opens the customer Postgres pool
(`dbproxy.NewPgProxy`), builds `daemon.New(cfg, pool)`, and calls `d.Run(ctx)`
(blocking, SIGINT/SIGTERM → cancel). `gw up`/`down` ([`up.go`](internal/gw/up.go))
install/start the same thing as a system service (`kardianos/service` via
`svcwrap`).

**Logs:** `gw run` opens `<stateDir>/daemon.log` and tees the daemon's status
lines to it via an `io.MultiWriter(stdout, file)` assigned to `daemon.Log`. The
service manager separately captures the process's stdout (e.g. launchd's
`StandardOutPath`). `gw logs` ([`logs.go`](internal/gw/logs.go)) prints the last
≤64 KiB of `daemon.log` (dropping a partial first line) then **follows** new
output (poll loop); `--audit` tails `<stateDir>/audit.log` instead.

### The `Daemon` and its reconnect loop

[`daemon/daemon.go`](internal/gw/daemon/daemon.go). `Daemon` keeps config, the
DB pool, and the connector resolver in **`atomic.Pointer`s** so `gw reload` can
swap them from the control-socket goroutine while `Run` reads them.

`Run(ctx)`:
- starts the control socket (below),
- loops `runOnce` with **exponential backoff + jitter** (cap from
  `Daemon.ReconnectMaxBackoffS`, default 60s),
- a `reloadCh` signal resets the backoff so a reload reconnects immediately.

`runOnce` builds a `dbproxy.PgProxy` + `dbproxy.Allowlist` from the current config,
dials a `tunnel.Client`, and registers the **tool handlers**: `query`, `status`,
`scan`, `inspect`, `configure`, `verify`, `introspect`. (When paused, `query`
returns an error.)

### The control socket

[`control/socket.go`](internal/gw/control/socket.go) (package `control`, **not**
`daemon/socket.go`) is a tiny JSON request/response protocol over a **Unix socket**
at `<cacheDir>/gw.sock` (`os.Chmod 0600`). `gw status|pause|resume|reload`
([`status.go`](internal/gw/status.go) `controlAction`, [`pause.go`](internal/gw/pause.go),
[`resume.go`](internal/gw/resume.go), [`reload.go`](internal/gw/reload.go)) send a
one-shot `control.Request`; the daemon dispatches in `handleControl`.

### `gw reload` semantics

`reload` ([`daemon.go`](internal/gw/daemon/daemon.go) `reload()`) re-reads
`gw.yaml`, rebuilds the DB pool **only if the URL changed** (and only after the new
pool connects cleanly, so a bad URL fails the reload without disturbing the live
connection), rebuilds the connector resolver if the enabled set changed, swaps the
config pointer, then signals `reloadCh` **and** cancels the live connection so the
loop redials immediately with the new config. Endpoint, allowlist, token-ref, and
connector edits all apply without a restart. (`TestReloadRereadsConfig`,
`TestReloadMissingPathErrors`.)

### `dbproxy` — allowlist / read-only / max-rows

[`dbproxy/allowlist.go`](internal/gw/dbproxy/allowlist.go) parses SQL (TiDB parser)
and enforces:
- **read-only**: only `SELECT` / `SetOpr` statements when `ReadOnly`,
- **table allowlist**: every referenced table must be in `Tables` (FQN
  `schema.table`, case-insensitive; bare names matched against the table portion),
- **blocked columns**: no column reference may match `BlockedColumns`
  (`table.column`).

[`dbproxy/postgres.go`](internal/gw/dbproxy/postgres.go) `PgProxy.Execute` runs the
SELECT with a query timeout (default 30s) and caps rows at `MaxRows` (default
10000). `Schema()` introspects only allowlisted tables and **excludes blocked
columns**, sharing `isColumnBlocked` with the query path so the schema cache can
never advertise a column the query path would reject.

### Connectors / resolver

[`connregistry/registry.go`](internal/gw/connregistry/registry.go) builds a
`connectors.TargetResolver` with the enabled connectors (`mqtt`, `modbus`, `opcua`,
`sonos`, `otel`; `generic` is always the fallback). The resolver dispatches the
`scan`/`inspect`/`configure`/`verify` tools to the right connector by service type.

### The tunnel client

[`tunnel/client.go`](internal/gw/tunnel/client.go) owns one WSS connection
(`github.com/coder/websocket`). It dials, sends an `auth` frame (token, gateway ID,
version, manifest), runs a heartbeat loop, and fans incoming `tool_request`
messages out to a worker pool that calls the registered handlers, replying
`tool_response`/`tool_error`. `Run` returns on transport error or ctx cancel — the
*daemon* owns the reconnect loop, not the client.

---

## 7. How to add a new command or group (recipe)

Two cases. Both are concrete and test-gated.

### Case A — expose a generated group

Use when the backend already has a tagged set of operations and you want them as a
CLI group. Example: exposing a hypothetical `widgets` tag as `widget`.

1. **Confirm the generated constructor exists.** After `make cli-codegen`, look for
   `func NewWidgetsCmd(...)` in `internal/platform/generated/widgets.gen.go`. (If
   the tag is new in the backend, run `make cli-codegen` first.)

2. **Add one `groupSpec` to `publicGroups`** in
   [`internal/platform/wire.go`](internal/platform/wire.go):

   ```go
   {new: generated.NewWidgetsCmd, use: "widget", why: "operators manage widgets"},
   ```

   - `new` = the generated constructor.
   - `use` = the singular CLI name (omit / `""` to keep the plural tag).
   - `why` = a one-line rationale (required by convention — this is the policy gate).

3. **Update the two policy tests** in
   [`internal/platform/wire_test.go`](internal/platform/wire_test.go):
   - add `"widget"` to the `want` slice in `TestAddTo_WiresIntendedGroups`,
   - bump the count in `TestPublicGroupsCount` (e.g. `want 20` → `want 21`).

4. **Verify:** `cd cli && go test ./internal/platform/` (or `make cli-test`).
   `go run ./cmd/yomiro widget --help` should list the generated subcommands.

### Case B — add a hand-written override command

Use for a command with no generated equivalent, or to replace a generated
subcommand. Example: `widget export`.

1. **Create `internal/platform/overrides/widget_export.go`** modeled on
   [`dashboard_render.go`](internal/platform/overrides/dashboard_render.go):

   ```go
   package overrides

   import "github.com/spf13/cobra"

   func init() {
       cmd := &cobra.Command{
           Use:   "export <widget-id>",
           Short: "Export a widget",
           Args:  cobra.ExactArgs(1),
           RunE: func(cmd *cobra.Command, args []string) error {
               // … implement …
               return nil
           },
       }
       Register("widget", cmd) // group key = the wired group name (singular)
   }
   ```

   The group key passed to `Register` must match the **wired** group name (the
   `use` value, e.g. `"widget"`), or the `skill` stub name for skill commands.

2. **No `wire.go` edit is needed** — `AddTo` already runs `overrides.AllInGroup` for
   every wired group (and `skill`). If the override's name matches a generated
   subcommand, the generated one is removed first (override wins).

3. **Verify with a hermetic test.** Add `internal/platform/overrides/widget_export_test.go`
   or extend `wire_test.go`. Pattern (see `TestAddTo_WiresAiConfigGroup` /
   `TestAddTo_WiresOnboardingGroups` in `wire_test.go`):

   ```go
   root := newRootForTest(t)
   group, _, _ := root.Find([]string{"widget"})
   cmd, _, err := group.Find([]string{"export"})
   if err != nil || cmd == nil || cmd == group {
       t.Fatalf("widget export not wired")
   }
   ```

   For commands that hit the API, point them at an `httptest.NewServer` and pass
   `--api-url <srv.URL>` (see `TestAddTo_FlagOverridesAPIURL`).

### Testing notes (cross-link)

The full test/build/live-login playbook lives in the **`cli-testing`** skill
(`.claude/skills/cli-testing/SKILL.md`). Key points relevant to adding commands:

- Tests are **hermetic**: mock the backend with `httptest.NewServer`; no stack, no
  Auth0.
- For anything touching the keyring (gw daemon tests), call `keyring.MockInit()` and
  `keyring.Set(...)` (see `daemon/daemon_test.go`).
- **macOS unix-socket path-length gotcha:** macOS caps unix-socket paths at ~104
  chars. The default `t.TempDir()` path can exceed that once `/yomiro/gw.sock` is
  appended, so daemon tests that bind the control socket create a **short**
  `/tmp/ygw*` dir and set `XDG_CACHE_HOME` to it instead of using `t.TempDir()`.
  See `TestRunLogsToProvidedWriter` in
  [`daemon/daemon_test.go`](internal/gw/daemon/daemon_test.go). The pure control
  package round-trip test (`control/socket_test.go`) uses `t.TempDir()` because its
  path stays short — follow whichever the neighboring test does.

---

## 8. The public mirror

[`.github/workflows/cli-mirror.yml`](../.github/workflows/cli-mirror.yml) rsyncs
`cli/` → the public repo `yomiroco/yomiro-cli` on every push to `main` that touches
CLI paths. It:

- copies `cli/` to the public repo root (`--delete`), **excluding `.git`,
  `.github`, and `bin/`**,
- rewrites the Go module path `github.com/hojland/yomiro/cli` →
  `github.com/yomiroco/yomiro-cli` (and the workflow `cli/go.mod` → `go.mod`,
  `workdir: cli` → `workdir: .`),
- copies `LICENSE` and the release workflow ([`cli-release.yml`](../.github/workflows/cli-release.yml),
  which is guarded to run only in the public repo),
- `go build ./...`s the rewritten tree before committing, so a broken mirror never
  lands.

> **Why the `cli-dev` dev skill lives at repo root, not under `cli/`:**
> `cli/.claude/skills/**` is **NOT** in the rsync exclude list, so it **ships
> publicly** (that's intentional — `yomiro-cli` is the user-facing skill, useful to
> downstream consumers). An *internal* dev skill must therefore live at
> **`.claude/skills/cli-dev/`** (repo root), which `cli-mirror` never copies.

---

## Cross-links

- **User skill:** [`cli/.claude/skills/yomiro-cli/SKILL.md`](.claude/skills/yomiro-cli/SKILL.md)
  — how to *use* the CLI.
- **Dev skill:** `.claude/skills/cli-dev/SKILL.md` (repo root) — discovery layer
  that points here.
- **Testing skill:** `.claude/skills/cli-testing/SKILL.md` (repo root) — running
  tests, building, live login flows.
