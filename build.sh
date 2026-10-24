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

# --- toolchains ---
# Go and Bun are looked up independently, in this order:
#   1. $GO_BIN_DIR / $BUN_BIN_DIR, if set
#   2. <repo>/toolchains/go/bin and <repo>/toolchains/bun/bin
#   3. whatever `go` / `bun` is already on PATH
# On this host they live apart (go under /mnt/software/toolchains, bun under
# /mnt/software/bun), so e.g.:
#   GO_BIN_DIR=/mnt/software/toolchains/go/bin BUN_BIN_DIR=/mnt/software/bun/bin ./build.sh
for d in "${GO_BIN_DIR:-$ROOT/toolchains/go/bin}" "${BUN_BIN_DIR:-$ROOT/toolchains/bun/bin}"; do
  [ -d "$d" ] && PATH="$d:$PATH"
done
export PATH

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "!!! $1 not found." >&2
    echo "!!! Install it, or point GO_BIN_DIR / BUN_BIN_DIR at the bin dirs." >&2
    exit 1
  }
}
need git
need go
need bun
need docker

# --- caches kept next to the repo, not in $HOME ---
export GOPATH="$CACHE/gopath"
export GOMODCACHE="$CACHE/gopath/pkg/mod"
export GOCACHE="$CACHE/gocache"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64
export BUN_INSTALL="${BUN_INSTALL:-$(dirname "$(dirname "$(command -v bun)")")}"
export BUN_INSTALL_CACHE_DIR="$CACHE/bun-cache"
export DISABLE_ESLINT_PLUGIN=true

# Report what we resolved, so a build log always says which toolchain ran.
echo "==> toolchain: $(go version)"
echo "==> toolchain: bun $(bun --version)"

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
# The runtime Dockerfile lives in deploy/ but its build context is runtime/
# (the binary + licenses). Keep the two apart so the binary is never committed.
cp "$ROOT/deploy/Dockerfile" "$OUT/Dockerfile"
docker build -t "$IMAGE" "$OUT"

echo "==> done: $IMAGE ($VER)"
echo "    now run: cd deploy && docker compose up -d"
