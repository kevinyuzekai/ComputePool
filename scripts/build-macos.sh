#!/usr/bin/env bash
# Cross-compile ComputePool for macOS.
# Usage: ARCH=arm64|amd64 ./scripts/build-macos.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-0.2.0}"
ARCH="${ARCH:-arm64}"
case "$ARCH" in
  arm64|aarch64) ARCH=arm64; GOARCH=arm64; ARCH_LABEL="Apple Silicon (arm64)" ;;
  amd64|x86_64|intel) ARCH=amd64; GOARCH=amd64; ARCH_LABEL="Intel (amd64 / x86_64)" ;;
  *)
    echo "error: ARCH must be arm64 or amd64 (got: $ARCH)" >&2
    exit 1
    ;;
esac

OUT_DIR="${OUT_DIR:-$ROOT/build/macos/$ARCH}"
BIN_NAME="ComputePool"
mkdir -p "$OUT_DIR"

echo "==> go test"
go test ./...

echo "==> build darwin/${GOARCH} (${ARCH_LABEL})"
CGO_ENABLED=0 GOOS=darwin GOARCH="$GOARCH" \
  go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
  -o "$OUT_DIR/${BIN_NAME}" ./cmd/computepool

echo "==> assemble .app bundle"
APP="$OUT_DIR/ComputePool.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$ROOT/packaging/macos/Info.plist" "$APP/Contents/Info.plist"
cp "$OUT_DIR/${BIN_NAME}" "$APP/Contents/MacOS/ComputePool"
chmod +x "$APP/Contents/MacOS/ComputePool"

if [[ -f "$ROOT/packaging/macos/AppIcon.icns" ]]; then
  cp "$ROOT/packaging/macos/AppIcon.icns" "$APP/Contents/Resources/AppIcon.icns"
fi

echo -n "APPL????" > "$APP/Contents/PkgInfo"

if [[ "$(uname -s)" == "Darwin" ]]; then
  echo "==> ad-hoc codesign"
  codesign --force --deep --sign - "$APP"
  codesign --force --sign - "$OUT_DIR/${BIN_NAME}"
  codesign -dv --verbose=2 "$APP" 2>&1 | head -20 || true
else
  echo "==> skip codesign (not Darwin); sign on a Mac before distributing"
fi

echo "Built (${ARCH}):"
echo "  $APP"
echo "  $OUT_DIR/${BIN_NAME}"
echo ""
echo "Run binary:  $OUT_DIR/${BIN_NAME} -open"
echo "Package DMG: ARCH=${ARCH} ./scripts/package-dmg.sh"
