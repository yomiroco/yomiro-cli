#!/usr/bin/env bash
set -euo pipefail

REPO="${YOMIRO_REPO:-yomiroco/yomiro-cli}"
VERSION="${INSTALL_VERSION:-latest}"
PREFIX="${PREFIX:-/usr/local}"
BIN_DIR="${BIN_DIR:-${PREFIX}/bin}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) echo "unsupported arch: ${ARCH}" >&2; exit 1 ;;
esac

if [[ "${VERSION}" == "latest" ]]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name"[^"]*"([^"]+)".*/\1/')"
fi

# Tag scheme in the public repo is plain semver (vX.Y.Z); strip the leading v
# for the artifact name (goreleaser names archives `yomiro_<semver>_<os>_<arch>.tar.gz`).
SEMVER="${VERSION#v}"

ARCHIVE="yomiro_${SEMVER}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"
SUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

echo "Downloading ${URL}"
curl -fsSL -o "${TMP}/${ARCHIVE}" "${URL}"

echo "Verifying checksum"
curl -fsSL -o "${TMP}/checksums.txt" "${SUMS_URL}"
( cd "${TMP}" && grep " ${ARCHIVE}\$" checksums.txt | shasum -a 256 -c - )

echo "Installing to ${BIN_DIR}"
tar -xzf "${TMP}/${ARCHIVE}" -C "${TMP}"
install -m 0755 "${TMP}/yomiro" "${BIN_DIR}/yomiro"

echo "✓ yomiro installed: $("${BIN_DIR}/yomiro" version)"
