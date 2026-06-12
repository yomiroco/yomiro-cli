# Onboarding a new plant with the Yomiro CLI

End-to-end setup of a new plant/tenant: the location hierarchy, users, device
groups, data sources, devices & cameras, capture rules, the gateway, and the first
dashboards. Read the main `SKILL.md` first for authentication and environment
targeting — this guide assumes you can already run authenticated `yomiro` commands
against the target environment.

The platform also has a guided, agent-driven onboarding flow; this guide is the
**CLI-native** path for operators who prefer to script setup or work headless.

## Prerequisites

- You are logged in as an operator with **write** scopes in the target tenant:
  ```bash
  yomiro auth login --scopes locations:write,devices:write,dashboards:write,data:write
  yomiro auth whoami      # confirm you're on the right tenant + environment
  ```
  (Use `--web` to pick scopes interactively, or check the web app's API-keys page
  for the scopes your role can grant.)
- Decide the environment up front and keep it consistent (`YOMIRO_API_URL`), since
  every resource you create lands in that one tenant/environment.

Throughout, capture the IDs that creates return so later steps can reference them:

```bash
PLANT_ID=$(yomiro location create --json-body '{"name":"Aarhus Plant","level":"plant"}' | jq -r '.id')
```

## Step 1 — Build the plant location hierarchy

Locations form a tree: a top-level **plant**, then **lines**, then **units** (the
exact level names are validated server-side — run `yomiro location create --skeleton`
or check the web app for the levels your tenant uses). Children link to parents via
`parent_id`.

```bash
# Plant (root: no parent_id)
PLANT_ID=$(yomiro location create --json-body '{
  "name": "Aarhus Plant", "level": "plant"
}' | jq -r '.id')

# Line under the plant
LINE_ID=$(yomiro location create --json-body "{
  \"name\": \"Assembly Line 1\", \"level\": \"line\", \"parent_id\": \"$PLANT_ID\"
}" | jq -r '.id')

# Unit under the line
UNIT_ID=$(yomiro location create --json-body "{
  \"name\": \"Station A\", \"level\": \"unit\", \"parent_id\": \"$LINE_ID\"
}" | jq -r '.id')

# Verify the tree
yomiro location get-tree --output table
```

`yomiro location list`, `get <id>`, `update <id>`, `get-devices <id>` round out the
group.

## Step 2 — Add users

User accounts are managed through **Auth0** (the identity provider) — that's where
passwords, SSO, and invitations live. Onboard users one of two ways:

- **Auth0 dashboard / your IdP:** create or invite each user in the Auth0 tenant (or
  via your SSO connection). On first sign-in to the Yomiro web app they are
  associated with this tenant. For bulk onboarding, use Auth0's bulk user import
  (a JSON/CSV import job in the Auth0 Management dashboard) or the Auth0 Management
  API — this is the supported path for adding many users from a spreadsheet.
- **Web app invite:** an admin invites teammates from the Yomiro web app's
  members/settings page.

Once users have signed in, inspect and manage them from the CLI:

```bash
yomiro user read --output table          # list tenant users
yomiro user read-by-id <userId>
yomiro user update <userId> --json-body '{"role":"operator"}'
```

> `yomiro user create` exists but expects an existing Auth0 identity (`auth0_sub`)
> and the `tenant_id`; it does not provision Auth0 accounts or send invitations.
> Prefer Auth0/web for actually onboarding people, then use `yomiro user` to inspect
> and adjust roles.

## Step 3 — Create device groups

Devices and AI/capture configuration attach to a **device group**, so create the
groups before the devices. Group devices the way you operate them (per line, per
cell, per camera cluster).

```bash
GROUP_ID=$(yomiro device-group create --json-body '{
  "name": "Line 1 Cameras", "description": "All cameras on Assembly Line 1"
}' | jq -r '.id')

yomiro device-group list --output table
```

## Step 4 — Register devices and cameras

Each device is created under a device group and pinned to a location. A camera is a
device with the appropriate `device_type`/`configuration` (run
`yomiro device create --skeleton` to see every field, then fill it in).

```bash
yomiro device create --skeleton > device.json     # inspect the template
# Edit device.json: name, device_group_id, location_id, device_type, configuration…

CAM_ID=$(yomiro device create --json-body "{
  \"name\": \"Cam A1\",
  \"device_group_id\": \"$GROUP_ID\",
  \"location_id\": \"$UNIT_ID\",
  \"device_type\": \"camera\"
}" | jq -r '.id')

yomiro device test-connection "$CAM_ID"      # verify connectivity
yomiro device check-health "$CAM_ID"
yomiro device list --output table
```

`device discover` can find devices on the network; `device trigger-sync` /
`get-sync-status` manage config push; `device push-model <deviceId> <modelId>`
deploys an AI model to an edge device.

## Step 5 — Configure capture rules (AI config)

Capture rules and detection settings live in the **AI config**, attached per device
group. Set what to detect and the capture behaviour with `ai-config`:

```bash
yomiro ai-config create-or-replace "$GROUP_ID" --skeleton > ai-config.json
# Edit: enabled, text_queries, model_pipeline (none|nanoowl_sam|sam3_lite|golden_state),
#       detection_source, thresholds, and the capture_config object (the capture rule).

yomiro ai-config create-or-replace "$GROUP_ID" --json-body '{
  "enabled": true,
  "model_pipeline": "sam3_lite",
  "text_queries": ["forklift", "person without helmet"],
  "capture_config": { ... }
}'
yomiro ai-config get "$GROUP_ID"
```

The inner `capture_config` shape is defined platform-side (a free-form object in the
API) — use `--skeleton` and the web app's capture-rule editor as the source of truth
for its keys. See the "Adding a capture rule" section in `SKILL.md`.

## Step 6 — Stand up the gateway (create `gw.yaml` *before* `gw up`)

The gateway runs on a machine inside the plant network and bridges a local database
to the platform. You must create its config file, `gw.yaml`, **before** starting the
daemon with `gw up`. Two ways:

**A. Let `gw init` write it (recommended).** This is the normal path — it generates
`~/.config/yomiro/gw.yaml`, mints the gateway token, and stores it in the keyring:

```bash
yomiro gw init --from-login \
  --gateway-id gw-aarhus-line1 \
  --db-url 'postgres://readonly:pass@localhost:5432/plantdb' \
  --endpoint wss://api.dev.yomiro.io/api/v1/gateway/ws \
  --allow-table public.production_runs --allow-table public.quality_checks \
  --block-column public.operators.personal_id
# → "✓ Wrote ~/.config/yomiro/gw.yaml" and stores the gateway:tunnel token
```

**B. Hand-write `gw.yaml`** if you need full control. Create
`~/.config/yomiro/gw.yaml`:

```yaml
platform:
  endpoint: wss://api.dev.yomiro.io/api/v1/gateway/ws
  token_ref: keyring:io.yomiro.gw/gw-aarhus-line1   # token stored under this keyring key
gateway:
  id: gw-aarhus-line1
  version: "0.1.0"
database:
  url: postgres://readonly:pass@localhost:5432/plantdb
  read_only: true
  allowed_tables:               # REQUIRED — an empty allowlist blocks every query
    - public.production_runs
    - public.quality_checks
  blocked_columns:
    - public.operators.personal_id
  max_rows_per_query: 10000
  max_connections: 5
  query_timeout_seconds: 30
connectors:
  enabled: [postgres]
daemon:
  auto_start: true
  reconnect_max_backoff_s: 60
  heartbeat_interval_s: 30
logging:
  level: info
```

If you hand-write the config, store the gateway token in the keyring under the
`token_ref` key (e.g. via `gw init --from-login` once, or your secret tooling).

Then start and verify the daemon:

```bash
yomiro gw up        # install + start as a service (or `gw run` to run in foreground)
yomiro gw status    # confirm connected; postgres connector up
```

See `SKILL.md` ("Gateway") for connecting/changing the local DB on a running daemon
(`gw reload`), the allowlist gotcha, and the full daemon lifecycle.

## Step 7 — Add the data sources

Register the databases the platform should query. A data source can connect
**directly** (host/port reachable from the platform) or be **proxied through the
gateway** you just set up (for a DB that only lives inside the plant network) by
setting `agent_id` to the gateway. Test and introspect before relying on it:

```bash
yomiro data-source create --skeleton > ds.json    # see all fields

# Direct connection
yomiro data-source create --json-body '{
  "name": "Plant warehouse (direct)",
  "ds_type": "postgresql",
  "host": "warehouse.db.internal", "port": 5432,
  "database_name": "warehouse", "username": "yomiro_ro", "password": "…",
  "ssl_mode": "require"
}'

# Or proxied through the gateway (DB only reachable inside the plant)
yomiro data-source create --json-body "{
  \"name\": \"Plant line DB (via gateway)\",
  \"ds_type\": \"postgresql\",
  \"agent_id\": \"<gatewayId>\"
}"

DS_ID=$(yomiro data-source list --output json | jq -r '.[0].id')
yomiro data-source test "$DS_ID"          # verify the connection works
yomiro data-source introspect "$DS_ID"    # discover schema/tables
```

`ds_type` is one of `postgresql | mysql | timescaledb`.

## Step 8 — Create the first dashboards

Build dashboards from scratch, from a template, or by importing an exported
definition (see "Migrating a dashboard between environments" in `SKILL.md` for the
cross-environment flow).

```bash
yomiro dashboard list-templates
yomiro dashboard import-template <templateKey>          # fastest start
# or author one:
yomiro dashboard create --skeleton > dash.json
yomiro dashboard create --json-body @dash.json
yomiro dashboard list --output table
```

## Verification checklist

```bash
yomiro location get-tree --output table     # plant → line → unit present
yomiro user read --output table             # expected users provisioned
yomiro device-group list --output table     # groups created
yomiro device list --output table           # devices/cameras registered + healthy
yomiro ai-config get <deviceGroupId>        # capture rules in place
yomiro gw status                            # gateway connected
yomiro data-source test <sourceId>          # data sources reachable
yomiro dashboard list --output table        # dashboards created
```

Once all of these look right, the plant is onboarded.
