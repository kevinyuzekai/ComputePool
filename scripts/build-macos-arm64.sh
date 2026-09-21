#!/usr/bin/env bash
set -euo pipefail
ARCH=arm64 "$(cd "$(dirname "$0")" && pwd)/build-macos.sh"
