#!/usr/bin/env bash
# Build the EaglerCMP daemon and (optionally) initialise the native runtime.
#   scripts/build.sh                       # build ./bin/eaglercmp for this machine
#   scripts/build.sh --target os/arch      # cross-compile ./bin/eaglercmp[.exe] for one target
#   scripts/build.sh --all                 # cross-compile into ./dist for every target
#   scripts/build.sh --init [source]       # build, then import the Eaglercraft client (or run init)
# Set VERSION (e.g. VERSION=v0.2.0) to stamp the launcher version.
# Set SKIP_CHECKS=1 to skip go vet / go test.
set -euo pipefail
cd "$(dirname "$0")/.."

command -v go >/dev/null || { echo "Go 1.22+ is required: https://go.dev/dl/" >&2; exit 1; }

VERSION="${VERSION:-}"
LDFLAGS="-s -w"
if [[ -n "$VERSION" ]]; then
  LDFLAGS+=" -X github.com/OnyxNeol/eaglercmp/config.LauncherVersion=${VERSION#v}"
fi

build() { # build <goos> <goarch> <output>
  echo "building $3 (${1}/${2})"
  local ldflags="$LDFLAGS"
  # Windows: GUI subsystem, so double-clicking opens only the game window.
  [[ "$1" == "windows" ]] && ldflags+=" -H windowsgui"
  CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -trimpath -ldflags "$ldflags" -o "$3" .
}

if [[ "${SKIP_CHECKS:-}" != "1" ]]; then
  go vet ./...
  go test ./...
fi

case "${1:-}" in
  --all)
    mkdir -p dist
    for target in linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64; do
      os="${target%/*}"; arch="${target#*/}"
      ext=""; [[ "$os" == "windows" ]] && ext=".exe"
      build "$os" "$arch" "dist/eaglercmp-${os}-${arch}${ext}"
    done
    ;;
  --target)
    target="${2:?usage: --target os/arch}"
    os="${target%/*}"; arch="${target#*/}"
    ext=""; [[ "$os" == "windows" ]] && ext=".exe"
    mkdir -p bin
    build "$os" "$arch" "bin/eaglercmp${ext}"
    ;;
  *)
    mkdir -p bin
    build "$(go env GOOS)" "$(go env GOARCH)" bin/eaglercmp
    if [[ "${1:-}" == "--init" ]]; then
      if [[ -n "${2:-}" ]]; then
        ./bin/eaglercmp import "$2"
      else
        ./bin/eaglercmp init
      fi
    fi
    ;;
esac
