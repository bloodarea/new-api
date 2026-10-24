#!/usr/bin/env bash
# Build new-api from upstream source + patches/, then package a small runtime
# image (only the embedded-everything binary) to keep Docker storage tiny.
#
# Layout this script expects (all paths relative to this repo):
#
#   <repo>/build.sh              this file
#   <repo>/patches/*.patch       applied to upstream in filename order
#   <repo>/deploy/               compose + runtime Dockerfile
#   <repo>/src/                  upstream checkout (NOT in git, see README)
#   <repo>/runtime/              build output (NOT in git)
#   <repo>/cache/                toolchain caches (NOT in git)
#
# Usage:  ./build.sh   then   cd deploy && docker compose up -d
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC="$ROOT/src"
OUT="$ROOT/runtime"
CACHE="$ROOT/cache"
IMAGE="${IMAGE:-new-api-local:local}"

# Upstream version this repo tracks. Must match patches/ base.
UPSTREAM_TAG="${UPSTREAM_TAG:-v1.0.0-rc.41}"
UPSTREAM_URL="${UPSTREAM_URL:-https://github.com/QuantumNous/new-api.git}"

# --- toolchains + caches kept next to the repo, not in $HOME ---
export PATH="$ROOT/toolchains/go/bin:$ROOT/toolchains/bun/bin:$PATH"
export GOPATH="$CACHE/gopath"
export GOMODCACHE="$CACHE/gopath/pkg/mod"
export GOCACHE="$CACHE/gocache"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64
export BUN_INSTALL="$ROOT/toolchains/bun"
export BUN_INSTALL_CACHE_DIR="$CACHE/bun-cache"
export DISABLE_ESLINT_PLUGIN=true

# --- 1/4: upstream checkout at the tracked tag ---
if [ ! -d "$SRC/.git" ]; then
  echo "==> [1/4] cloning $UPSTREAM_URL at $UPSTREAM_TAG"
  git clone --branch "$UPSTREAM_TAG" --depth 1 "$UPSTREAM_URL" "$SRC"
fi

cd "$SRC"
VER=$(cat VERSION 2>/dev/null || echo "$UPSTREAM_TAG")

# --- 2/4: apply patches ---
echo "==> [2/4] applying patches for $VER"
shopt -s nullglob
for p in "$ROOT"/patches/*.patch; do
  if git apply --check "$p" 2>/dev/null; then
    git apply "$p"
    echo "    applied $(basename "$p")"
  elif git apply --reverse --check "$p" 2>/dev/null; then
    echo "    already applied $(basename "$p")"
  else
    echo "!!! $(basename "$p") does not apply cleanly." >&2
    echo "!!! Upstream may have moved; rebase the patch before building." >&2
    exit 1
  fi
done
VER=$(cat VERSION 2>/dev/null || echo "$UPSTREAM_TAG")

# --- 3/4: frontend + backend ---
echo "==> [3/4] frontend build ($VER)"
cd "$SRC/web"
[ -d node_modules ] || bun install --frozen-lockfile
VITE_REACT_APP_VERSION="$VER" bun run build

echo "    backend build"
cd "$SRC"
go build -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$VER'" -o "$OUT/new-api" .

# --- 4/4: runtime image ---
echo "==> [4/4] runtime image ($IMAGE)"
mkdir -p "$OUT/licenses"
cp "$SRC/LICENSE" "$SRC/NOTICE" "$SRC/THIRD-PARTY-LICENSES.md" "$OUT/licenses/"
docker build -t "$IMAGE" "$OUT"

echo "==> done: $IMAGE ($VER)"
echo "    now run: cd deploy && docker compose up -d"
