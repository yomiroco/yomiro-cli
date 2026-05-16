# Installing the Yomiro CLI

`yomiro` ships as a single static binary for macOS, Linux, and Windows
(`amd64` + `arm64`, except Windows which is `amd64`-only). Every release is
[cosign](https://github.com/sigstore/cosign)-signed and ships a CycloneDX
SBOM.

Pick the method that fits your environment:

- [Homebrew (macOS / Linux)](#homebrew-macos--linux) — recommended for
  workstations
- [Docker](#docker) — recommended for running the gateway daemon
- [Pre-built binaries](#pre-built-binaries) — air-gapped / no package manager
- [`go install`](#go-install) — if you already have a Go toolchain
- [Build from source](#build-from-source)
- [Shell completions](#shell-completions)
- [Verifying release artifacts](#verifying-release-artifacts)
- [Upgrading](#upgrading)
- [Uninstalling](#uninstalling)

---

## Homebrew (macOS / Linux)

```sh
brew install yomiroco/yomiro/yomiro
```

This taps `yomiroco/homebrew-yomiro` and installs the latest non-prerelease
formula. To upgrade later, `brew upgrade yomiro`.

If you prefer an explicit tap:

```sh
brew tap yomiroco/yomiro
brew install yomiro
```

> Prerelease tags (e.g. `v0.1.0-rc1`) intentionally do **not** update the
> Homebrew formula — Homebrew always tracks the latest stable release.

## Docker

The gateway daemon is published as a multi-arch distroless image on GHCR:

```sh
docker pull ghcr.io/yomiroco/yomiro:latest
```

The image's default command runs the gateway daemon (`yomiro gw run`). Run
any subcommand by overriding it:

```sh
# Show version
docker run --rm ghcr.io/yomiroco/yomiro:latest version

# Run the gateway daemon (default CMD)
docker run --rm ghcr.io/yomiroco/yomiro:latest

# Pin to a specific release
docker pull ghcr.io/yomiroco/yomiro:0.0.1
```

The image is `distroless` (no shell), runs as a non-root user, and is built
for `linux/amd64` and `linux/arm64`.

## Pre-built binaries

1. Go to [Releases](https://github.com/yomiroco/yomiro-cli/releases) and
   download the archive for your OS/arch, e.g.
   `yomiro_<version>_darwin_arm64.tar.gz`.
2. (Recommended) [verify the download](#verifying-release-artifacts).
3. Extract and place the binary on your `$PATH`:

   ```sh
   tar -xzf yomiro_<version>_<os>_<arch>.tar.gz
   sudo install -m 0755 yomiro /usr/local/bin/yomiro
   ```

   On Windows, unzip `yomiro_<version>_windows_amd64.zip` and move
   `yomiro.exe` somewhere on your `PATH`.

4. Confirm it works:

   ```sh
   yomiro version
   ```

## `go install`

Requires Go 1.25+. Installs into `$(go env GOBIN)` (or `$(go env GOPATH)/bin`):

```sh
go install github.com/yomiroco/yomiro-cli/cmd/yomiro@latest
```

Pin to a release instead of `@latest` for reproducible installs:

```sh
go install github.com/yomiroco/yomiro-cli/cmd/yomiro@v0.0.1
```

> Builds installed this way report `version dev` from `yomiro version`
> because release version metadata is injected by the release pipeline's
> linker flags, not by `go install`.

## Build from source

```sh
git clone https://github.com/yomiroco/yomiro-cli
cd yomiro-cli
go build -o yomiro ./cmd/yomiro
./yomiro version
```

Requires Go 1.25+ (see `go.mod` for the exact version).

## Shell completions

`yomiro` generates completion scripts for bash, zsh, fish, and PowerShell.

**bash** (Linux):

```sh
yomiro completion bash | sudo tee /etc/bash_completion.d/yomiro > /dev/null
```

**zsh** (add to a directory on your `$fpath`):

```sh
yomiro completion zsh > "${fpath[1]}/_yomiro"
# then restart your shell
```

**fish**:

```sh
yomiro completion fish > ~/.config/fish/completions/yomiro.fish
```

**PowerShell** (add to your `$PROFILE`):

```powershell
yomiro completion powershell | Out-String | Invoke-Expression
```

Run `yomiro completion <shell> --help` for shell-specific notes.

## Verifying release artifacts

Each release publishes a cosign-signed `checksums.txt` (with its `.pem`
certificate and `.sig` signature) plus a per-archive CycloneDX SBOM.

Verify the checksums file is authentically from this repo's release
pipeline (keyless / Sigstore):

```sh
cosign verify-blob \
  --certificate checksums.txt.pem \
  --certificate-identity-regexp 'https://github.com/yomiroco/yomiro-cli/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --signature checksums.txt.sig \
  checksums.txt
```

Then verify your downloaded archive against it:

```sh
sha256sum --ignore-missing -c checksums.txt
```

(`shasum -a 256 -c checksums.txt` on macOS.)

## Upgrading

| Method        | Command                                             |
| ------------- | --------------------------------------------------- |
| Homebrew      | `brew upgrade yomiro`                                |
| Docker        | `docker pull ghcr.io/yomiroco/yomiro:latest`        |
| `go install`  | re-run `go install …/cmd/yomiro@latest`             |
| Binary        | download the newer archive and replace the binary   |

## Uninstalling

| Method        | Command                                             |
| ------------- | --------------------------------------------------- |
| Homebrew      | `brew uninstall yomiro && brew untap yomiroco/yomiro` |
| Binary        | `sudo rm /usr/local/bin/yomiro`                     |
| `go install`  | `rm $(go env GOPATH)/bin/yomiro`                    |

Stored credentials live in your OS keychain (via `zalando/go-keyring`); run
`yomiro logout` before uninstalling to remove them.

---

Next steps: see the [Quickstart](README.md#quickstart) in the README.
