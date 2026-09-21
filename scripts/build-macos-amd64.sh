#!/usr/bin/env bash
set -euo pipefail
ARCH=amd64 "$(cd "$(dirname "$0")" && pwd)/build-macos.sh"
