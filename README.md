# new-api-deploy

Deployment configuration for a self-hosted [new-api](https://github.com/QuantumNous/new-api)
instance: the patches we carry on top of upstream, the build script, and the
compose stack.

**Upstream source is not vendored here.** This repo holds only what turns
upstream into *our* build — a few hundred KB instead of ~80 MB of someone
else's history.

## What's in here

| Path | Purpose |
|---|---|
| `patches/*.patch` | Our changes on top of upstream, applied in filename order |
| `build.sh` | Clones upstream, applies patches, builds, packages the runtime image |
| `deploy/docker-compose.yml` | The stack: new-api + postgres + redis |
| `deploy/Dockerfile` | Minimal runtime image (binary + licenses only) |
| `.env.example` | Config template — copy to `.env`, fill in secrets |

## Patches

Base: **v1.0.0-rc.41**

| Patch | What it changes |
|---|---|
| `01-channel-site-icon.patch` | Channel site/icon URL feature: a channel can carry a website URL, and the dashboard fetches and shows its favicon. Adds `GET /api/channel/fetch_icon` (permission `ChannelOperate`), a `SiteURL` field on channel settings, and the drawer/column UI. |
| `02-web-cache-ratelimit.patch` | Two fixes for the dashboard loading stale or broken. **Cache**: only content-hashed `/static/*` assets get a long `max-age`; everything else — `index.html` and SPA fallbacks — gets `no-store`, so a browser never keeps HTML pointing at a previous build's chunks. **Rate limit**: `static.Serve` now runs *before* `GlobalWebRateLimit`, because one dashboard load requests dozens of chunks and counting them against the per-IP budget made a normal reload trip 429 / `ChunkLoadError`. |
| `03-docker-mirror.patch` | Points the build stages at a China-reachable mirror (`docker.m.daocloud.io`) and sets `GOPROXY`/`BUN_CONFIG_REGISTRY` to domestic mirrors. Also writes `VERSION`. Cosmetic for anyone outside China — drop it if you build elsewhere. |

Applying all three to upstream `v1.0.0-rc.41` reproduces the tree we build
byte for byte.

## Build

Prerequisites: Docker, Go (matching upstream's `go.mod`), Bun, git.

```sh
./build.sh
```

It will:

1. `git clone --branch v1.0.0-rc.41 --depth 1` into `./src/` if absent
2. apply every patch in `patches/`, refusing to build if one no longer applies
3. build the frontend (Bun) and backend (Go) into `./runtime/`
4. `docker build` the runtime image as `new-api-local:local`

Toolchains and caches live under `./cache/` and `./toolchains/`, both
gitignored, so nothing pollutes `$HOME`.

**Memory note:** the Go build is the heaviest step. On a low-RAM box
(≤4 GB) cap it, e.g. `GOMAXPROCS=2 ./build.sh`, or build on a stronger
machine and ship the image.

## Deploy

```sh
cd deploy
cp ../.env.example ../.env    # then fill in the three secrets
docker compose --env-file ../.env up -d
```

Generate secrets with `openssl rand -hex 32`.

The service listens on `3000`. Data lives in the `pg_data` volume (Postgres)
and `deploy/data` (uploads); logs in `deploy/logs`.

## Updating upstream

```sh
cd src && git fetch --unshallow && git checkout v1.0.0-rc.XX
```

Then in this repo:

```sh
for p in patches/*.patch; do git -C src apply --check "$p" || echo "CONFLICT: $p"; done
```

Rebase the conflicting patch, bump `UPSTREAM_TAG` in `build.sh` and the base
version in this README, then rebuild.

Patches are kept in three separate files on purpose: when upstream moves, the
Dockerfile/mirror patch usually still applies, and only the Go/frontend ones
need attention.

## Rolling back

```sh
docker tag new-api-local:local new-api-local:broken   # keep the bad one
docker tag new-api-local:<good> new-api-local:local
cd deploy && docker compose up -d
```

Postgres data is untouched by an image swap, but note that **a new-api
downgrade is not always schema-safe** — `AutoMigrate` adds columns and never
removes them, so going back a version usually works, while going forward and
then back across a schema change may not. Dump the database before a
version jump.

## License

Upstream new-api is **AGPL-3.0**. The patches here are derivative work and
carry the same license. This repository contains no upstream source; `src/`
is fetched at build time and stays out of git.
