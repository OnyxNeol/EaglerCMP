#!/usr/bin/env bash
# Package a built binary with README.md and NOTICE.md.
#   scripts/package.sh <os> <arch> <version>
# Reads bin/eaglercmp[.exe]; writes dist/eaglercmp-<version>-<os>-<arch>.{zip,tar.gz}
# (zip for Windows, tar.gz otherwise) and prints the archive path.
set -euo pipefail
cd "$(dirname "$0")/.."

os="${1:?os}"; arch="${2:?arch}"; version="${3:?version}"
ext=""; [[ "$os" == "windows" ]] && ext=".exe"
bin="bin/eaglercmp${ext}"
[[ -f "$bin" ]] || { echo "missing $bin; run scripts/build.sh --target $os/$arch first" >&2; exit 1; }

name="eaglercmp-${version}-${os}-${arch}"
stage="dist/${name}"
rm -rf "$stage"
mkdir -p "$stage"
cp "$bin" "$stage/"
cp README.md NOTICE.md "$stage/"
chmod +x "$stage/eaglercmp${ext}"

(
  cd dist
  if [[ "$os" == "windows" ]]; then
    rm -f "${name}.zip"
    if command -v zip >/dev/null; then
      zip -qr "${name}.zip" "$name"
    else
      7z a -tzip -bso0 "${name}.zip" "$name"
    fi
    echo "dist/${name}.zip"
  else
    tar -czf "${name}.tar.gz" "$name"
    echo "dist/${name}.tar.gz"
  fi
)
rm -rf "$stage"
