#!/bin/sh
# install.sh — install the Yomiro CLI (`yomiro`) from GitHub Releases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/yomiroco/yomiro-cli/main/install.sh | sh
#
# Environment overrides:
#   YOMIRO_VERSION       release tag to install (default: latest, e.g. v0.0.1)
#   YOMIRO_INSTALL_DIR   install directory (default: /usr/local/bin, or
#                        ~/.local/bin when /usr/local/bin is not writable)
#   YOMIRO_NO_VERIFY     set to any value to skip checksum verification
#
# Downloads the right archive for your OS/arch, verifies it against the
# cosign-signed checksums.txt, and installs the binary onto your PATH. For
# Homebrew, signature (cosign) verification, Docker, and air-gapped installs,
# see INSTALL.md.
set -eu

REPO="yomiroco/yomiro-cli"
BINARY="yomiro"

err() { printf 'install.sh: %s\n' "$1" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || err "required tool '$1' not found on PATH"; }

need uname
need tar
if command -v curl >/dev/null 2>&1; then
  dl() { curl -fsSL "$1" -o "$2"; }
  fetch() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
  dl() { wget -qO "$2" "$1"; }
  fetch() { wget -qO- "$1"; }
else
  err "need curl or wget to download"
fi

# --- detect OS/arch, mapped to goreleaser's name_template ---
os=$(uname -s)
case "$os" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) err "unsupported OS '$os' — see INSTALL.md for pre-built binaries (Windows: use the zip from the Releases page)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) err "unsupported arch '$arch'" ;;
esac

# --- resolve the version tag ---
tag="${YOMIRO_VERSION:-}"
if [ -z "$tag" ]; then
  # GitHub redirects /releases/latest to /releases/tag/<tag>; parse the final URL.
  tag=$(fetch "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
  [ -n "$tag" ] || err "could not determine the latest release tag — set YOMIRO_VERSION explicitly"
fi
version="${tag#v}" # goreleaser .Version drops the leading v in asset names

archive="${BINARY}_${version}_${os}_${arch}.tar.gz"
base="https://github.com/${REPO}/releases/download/${tag}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

printf 'Downloading %s (%s)...\n' "$archive" "$tag"
dl "${base}/${archive}" "${tmp}/${archive}" || err "download failed: ${base}/${archive}"

# --- verify against checksums.txt unless opted out ---
if [ -z "${YOMIRO_NO_VERIFY:-}" ]; then
  if dl "${base}/checksums.txt" "${tmp}/checksums.txt" 2>/dev/null; then
    if command -v sha256sum >/dev/null 2>&1; then
      sumcmd="sha256sum"
    elif command -v shasum >/dev/null 2>&1; then
      sumcmd="shasum -a 256"
    else
      sumcmd=""
    fi
    if [ -n "$sumcmd" ]; then
      ( cd "$tmp" && $sumcmd --ignore-missing -c checksums.txt >/dev/null 2>&1 ) \
        || err "checksum verification failed for ${archive}"
      printf 'Checksum OK.\n'
    else
      printf 'warning: no sha256 tool found; skipping checksum verification\n' >&2
    fi
  else
    printf 'warning: could not fetch checksums.txt; skipping verification\n' >&2
  fi
fi

tar -xzf "${tmp}/${archive}" -C "$tmp"
[ -f "${tmp}/${BINARY}" ] || err "archive did not contain a '${BINARY}' binary"
chmod +x "${tmp}/${BINARY}"

# --- pick an install dir ---
dir="${YOMIRO_INSTALL_DIR:-/usr/local/bin}"
if [ ! -d "$dir" ] || [ ! -w "$dir" ]; then
  if [ "$dir" = "/usr/local/bin" ]; then
    dir="${HOME}/.local/bin"
    mkdir -p "$dir"
    printf 'note: /usr/local/bin not writable; installing to %s\n' "$dir" >&2
  else
    err "install dir '$dir' is not writable"
  fi
fi

mv "${tmp}/${BINARY}" "${dir}/${BINARY}"
printf 'Installed %s to %s\n' "$BINARY" "${dir}/${BINARY}"

case ":${PATH}:" in
  *":${dir}:"*) ;;
  *) printf 'note: %s is not on your PATH — add it, e.g. export PATH="%s:$PATH"\n' "$dir" "$dir" >&2 ;;
esac

"${dir}/${BINARY}" version || true
