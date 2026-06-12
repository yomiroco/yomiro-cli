# yomiro

[![Release](https://github.com/yomiroco/yomiro-cli/actions/workflows/release.yml/badge.svg)](https://github.com/yomiroco/yomiro-cli/actions/workflows/release.yml)

Yomiro's command-line client: a gateway daemon for on-prem device/asset
connectivity and a control surface for the Yomiro platform.

> Public release mirror of the CLI built inside the Yomiro monorepo. Issues and
> PRs against the CLI itself are welcome here; platform issues belong in the
> main product channels.

## Install

```sh
brew install yomiroco/yomiro/yomiro
```

Or, without a package manager (macOS / Linux):

```sh
curl -fsSL https://raw.githubusercontent.com/yomiroco/yomiro-cli/main/install.sh | sh
```

Other methods — Docker, pre-built binaries, `go install`, build from
source, shell completions, signature verification — are documented in
**[INSTALL.md](INSTALL.md)**.

## Quickstart

```sh
yomiro auth login            # device-code flow against Auth0
yomiro auth whoami           # confirm you're authenticated
yomiro --help                # list commands
```

`yomiro auth login` opens a browser for the Auth0 device-code grant. The resulting
access token is stored in your OS keychain (via `zalando/go-keyring`).

### Choosing scopes: `--web` vs `--scopes`

`yomiro auth login` mints a scoped API key. How you choose those scopes
depends on whether a human is present:

- **Interactive shells — use `--web` (recommended).** It opens the platform
  web app to pick the key's scopes and lifetime before it's minted, so you
  grant exactly what you need. `--web` needs a resolvable frontend, so target
  a public API (e.g. `--api-url https://api.dev.yomiro.io`).
- **Headless / CI — use the default silent mint with `--scopes`.** No browser
  is involved; pass scopes explicitly and select the tenant via the
  `YOMIRO_API_URL` / `YOMIRO_AUTH0_*` env vars below
  (`yomiro auth login --scopes agents:read,dashboards:read`).

### Pointing at a non-prod tenant

```sh
export YOMIRO_API_URL=https://api.dev.yomiro.io
export YOMIRO_AUTH0_CLIENT_ID=<dev-client-id>
export YOMIRO_AUTH0_AUDIENCE=https://api.dev.yomiro.io
yomiro auth login
```

The compiled-in defaults target prod; dev requires explicit overrides so that
production-tagged binaries never silently issue tokens against the dev tenant.

## Verifying release artifacts

Every release archive is signed with [cosign](https://github.com/sigstore/cosign)
(keyless) and the `checksums.txt` is signed alongside the binaries. To verify:

```sh
cosign verify-blob \
  --certificate checksums.txt.pem \
  --certificate-identity-regexp 'https://github.com/yomiroco/yomiro-cli/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --signature checksums.txt.sig \
  checksums.txt
```

A CycloneDX SBOM is attached to each GitHub release.

## License

Apache-2.0 — see [LICENSE](LICENSE).
