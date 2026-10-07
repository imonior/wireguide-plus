#!/usr/bin/env bash
# Downloads the official wintun.dll, verifies SHA256, and copies the arch-
# appropriate DLL plus the Prebuilt Binaries License that governs it into the
# given output directory.
#
# Usage: vendor-wintun.sh <version> <sha256> <arch> <out_dir>
set -euo pipefail

if [ "$#" -ne 4 ]; then
    echo "usage: $0 <version> <sha256> <arch> <out_dir>" >&2
    exit 2
fi

VERSION="$1"
EXPECT="$2"
ARCH="$3"
OUT_DIR="$4"

case "$ARCH" in
    386) DLL_ARCH="x86" ;;
    *)   DLL_ARCH="$ARCH" ;;
esac

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
ZIP="$TMP/wintun.zip"

echo "Downloading wintun-$VERSION.zip"
curl -fsSL "https://www.wintun.net/builds/wintun-$VERSION.zip" -o "$ZIP"

if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL=$(sha256sum "$ZIP" | awk '{print $1}')
else
    ACTUAL=$(shasum -a 256 "$ZIP" | awk '{print $1}')
fi
if [ "$ACTUAL" != "$EXPECT" ]; then
    echo "wintun.zip SHA256 mismatch. expected=$EXPECT actual=$ACTUAL" >&2
    exit 1
fi

unzip -q -o "$ZIP" -d "$TMP/extract"
SRC="$TMP/extract/wintun/bin/$DLL_ARCH/wintun.dll"
if [ ! -f "$SRC" ]; then
    echo "wintun.dll not found at $SRC (unknown arch: $DLL_ARCH)" >&2
    exit 1
fi

# The DLL is licensed, not sold, under the Prebuilt Binaries License shipped
# inside the same zip; it must travel with every redistribution, so it is
# vendored next to the DLL instead of being dropped with the extraction dir.
LICENSE_SRC="$TMP/extract/wintun/LICENSE.txt"
if [ ! -f "$LICENSE_SRC" ]; then
    echo "wintun/LICENSE.txt not found at $LICENSE_SRC (the license must ship with the DLL)" >&2
    exit 1
fi

mkdir -p "$OUT_DIR"
OUT_NAME="wintun-$DLL_ARCH.dll"
cp "$SRC" "$OUT_DIR/$OUT_NAME"
cp "$LICENSE_SRC" "$OUT_DIR/wintun-LICENSE.txt"
echo "Bundled $DLL_ARCH driver as $OUT_NAME and its license as wintun-LICENSE.txt to $OUT_DIR"
