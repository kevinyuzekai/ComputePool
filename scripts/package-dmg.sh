#!/usr/bin/env bash
# Create a distributable .dmg containing ComputePool.app
# MUST run on macOS (requires hdiutil).
# Usage: ARCH=arm64|amd64 ./scripts/package-dmg.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

VERSION="${VERSION:-0.1.0}"
ARCH="${ARCH:-arm64}"
case "$ARCH" in
  arm64|aarch64) ARCH=arm64 ;;
  amd64|x86_64|intel) ARCH=amd64 ;;
  *)
    echo "error: ARCH must be arm64 or amd64 (got: $ARCH)" >&2
    exit 1
    ;;
esac

OUT_DIR="${OUT_DIR:-$ROOT/build/macos/$ARCH}"
APP="$OUT_DIR/ComputePool.app"
DMG_NAME="ComputePool-${VERSION}-${ARCH}.dmg"
DMG_PATH="$OUT_DIR/$DMG_NAME"
VOL_NAME="ComputePool"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "error: package-dmg.sh must run on macOS (hdiutil not available on $(uname -s))." >&2
  echo "On this Linux build box we only prepare the .app layout via build-macos.sh." >&2
  echo "On your Mac:" >&2
  echo "  ARCH=arm64 ./scripts/build-macos.sh && ARCH=arm64 ./scripts/package-dmg.sh" >&2
  echo "  ARCH=amd64 ./scripts/build-macos.sh && ARCH=amd64 ./scripts/package-dmg.sh" >&2
  exit 1
fi

if [[ ! -d "$APP" ]]; then
  echo "error: $APP not found. Run ARCH=${ARCH} ./scripts/build-macos.sh first." >&2
  exit 1
fi

STAGE="$OUT_DIR/dmg-stage"
rm -rf "$STAGE" "$DMG_PATH"
mkdir -p "$STAGE"
cp -R "$APP" "$STAGE/"
ln -s /Applications "$STAGE/Applications"

STAGE_APP="$STAGE/ComputePool.app"
echo "==> ad-hoc codesign (stage, ${ARCH})"
codesign --force --deep --sign - "$STAGE_APP"

ARCH_HINT="Apple Silicon (arm64)"
if [[ "$ARCH" == "amd64" ]]; then
  ARCH_HINT="Intel (x86_64 / amd64)"
fi

cat > "$STAGE/使用说明.txt" << TXT
ComputePool 设备算力聚合 v${VERSION} — ${ARCH_HINT}

1. 将 ComputePool.app 拖到「应用程序」文件夹
2. 首次打开若被拦截：系统设置 → 隐私与安全性 → 仍要打开
   或在终端执行：xattr -cr /Applications/ComputePool.app
3. 本应用没有原生窗口；启动后请用浏览器打开 http://127.0.0.1:9797
4. 在 iPhone / iPad 安装 ComputePoolWorker，粘贴加入 URL 保持连接
5. 在控制面板点「算力对比」查看 Mac 单机 vs Mac+设备加速

请下载与本机芯片匹配的 DMG：
  · Apple Silicon (M1/M2/M3/…) → ComputePool-*-arm64.dmg
  · Intel Mac → ComputePool-*-amd64.dmg

本工具是可拆分任务的 LAN 编排器，不是透明 OS CPU / GPU 融合。
TXT

echo "==> hdiutil create $DMG_PATH"
hdiutil create -volname "$VOL_NAME" -srcfolder "$STAGE" -ov -format UDZO "$DMG_PATH"
rm -rf "$STAGE"
echo "DMG ready: $DMG_PATH"
ls -lh "$DMG_PATH"
