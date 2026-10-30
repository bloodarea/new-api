# new-api — custom build

A customized [new-api](https://github.com/QuantumNous/new-api) for a private
instance. Upstream's [README](https://github.com/QuantumNous/new-api#readme) is
the reference for what the product is and how it works; this branch only covers
what we changed, how we build it, and how we run it.

This is not a contribution back to upstream — it is a personal build.

> 中文文档见 [README_ZH.md](README_ZH.md)

```sh
git clone -b deploy-tooling https://github.com/bloodarea/new-api.git new-api-deploy
```

## What's different from upstream

Base: `v1.0.0-rc.41` (upstream commit `2035a82`), three patches applied in
filename order.

| Patch | Change |
|---|---|
| `01-channel-site-icon` | Channels can carry a website URL, and the dashboard fetches that site's favicon and shows it in the channel list. Adds `GET /api/channel/fetch_icon` (`ChannelOperate`), `SiteURL`/`IconURL` in channel settings (settings JSON only, no schema change), `<link rel="…icon…">` parsing with an `<origin>/favicon.ico` fallback, SSRF-protected HTTP client, plus drawer/column UI and a unit test. |
| `02-web-cache-ratelimit` | Two dashboard fixes. **Cache**: only content-hashed `/static/*` gets a long `max-age`; `index.html` and SPA fallbacks get `no-store`, so a browser never keeps HTML pointing at a previous build's chunks. **Rate limit**: `static.Serve` runs before `GlobalWebRateLimit`, because one dashboard load pulls dozens of chunks and used to trip 429 / `ChunkLoadError` on a normal reload. |
| `03-docker-mirror` | Build-time only: points the base images and the package managers at China mirrors (`docker.m.daocloud.io`, npmmirror, goproxy.cn) and writes `VERSION`. Does not change the produced image — CI skips it and passes `BASE_REGISTRY=` empty instead. |

CI applies `01` and `02` only; `03` exists for local builds on a China host.

## Branches

Everything lives in this one fork:

| Branch | Purpose |
|---|---|
| `main` | Upstream mirror + `.github/workflows/build-image.yml`. GitHub only lists workflows from the default branch, so this is what gives the Actions tab its "Run workflow" button. |
| `deploy/rc.41` | The tree CI compiles: upstream tag + `01` + `02` + `VERSION` + `deploy/Dockerfile`. Generated — do not edit by hand. |
| `deploy-tooling` | This branch: patches, build script, compose stack, docs. |

## Build

**GitHub Actions (the default).** Actions → *Build patched image* → Run
workflow, with the defaults. It builds the frontend (Bun 1.4.0) and backend
(Go 1.26.1), packages the runtime image and pushes it:

```
ghcr.io/bloodarea/new-api:v1.0.0-rc.41
ghcr.io/bloodarea/new-api:latest
```

The bare binary is uploaded as an artifact (14 days). No secret is needed — the
workflow pushes with its own `GITHUB_TOKEN`. The package is public, so the run
host can pull without logging in.

### Do not build locally

Local builds are **disabled by default**. A frontend build plus `go build` pins
every core and peaks at several GB of RAM; run on a host that is also serving
traffic, that has frozen one of our machines. Use GitHub Actions — it is free
here and takes about 2.5 minutes.

`build.sh` refuses to start unless you pass `ALLOW_LOCAL_BUILD=1`, and even then
it aborts when the machine is below these minimums:

| Resource | Minimum | Why |
|---|---|---|
| CPU | 4 cores (8 recommended) | the Bun build and the Go build both parallelise |
| RAM | 8 GB available (16 GB total) | `go build` peaks at several GB; below this the host swaps and stalls |
| Disk | 20 GB free | node_modules ~1.5 GB, Go module cache ~3 GB, build cache ~2 GB, image ~1 GB |
| Host | a development machine | never a host that runs the stack |
| Tools | Go 1.26.x, Bun 1.4.0, Docker | the versions CI uses |

If a local build is genuinely needed and the machine meets all of that:

```sh
ALLOW_LOCAL_BUILD=1 ./build.sh       # aborts if RAM or disk are short
ALLOW_LOCAL_BUILD=force ./build.sh   # skips the resource check
```

Toolchains are looked up in `$GO_BIN_DIR` / `$BUN_BIN_DIR`, then `toolchains/`,
then `PATH`. Caches go to `cache/`, never `$HOME`.

## Deploy

```sh
git clone -b deploy-tooling https://github.com/bloodarea/new-api.git new-api-deploy
cd new-api-deploy

docker pull ghcr.io/bloodarea/new-api:v1.0.0-rc.41
docker tag  ghcr.io/bloodarea/new-api:v1.0.0-rc.41 new-api-local:local

cd deploy
cp ../.env.example ../.env      # fill in the secrets
docker compose --env-file ../.env up -d
```

- Service on port `3000`; Postgres data in the `pg_data` volume, uploads in
  `deploy/data`, logs in `deploy/logs`.
- `SESSION_SECRET` signs session cookies — changing it logs everyone out.
  Generate secrets with `openssl rand -hex 32`.
- The compose file sets `HTTP_PROXY`/`HTTPS_PROXY` for the container, defaulting
  to the host's port 7890. Set `HTTP_PROXY_URL=` empty in `.env` if the host has
  direct internet.

## Changing something

```sh
# 1. edit patches/*.patch in this branch
# 2. regenerate the branch CI builds (and the workflow on main)
FORK_CLONE=~/SourceCodeLibrary/Projects/new-api ./fork-branch.sh
# 3. Actions -> Build patched image -> Run workflow
```

`fork-branch.sh` resets `main` and `deploy/rc.41` in the clone it points at, so
give it a clean clone of the fork.

Upgrading upstream: bump `UPSTREAM_TAG` (in `build.sh` and in the script), rebase
the patches that no longer apply, regenerate.

Rolling back:

```sh
docker tag new-api-local:local new-api-local:broken   # keep the bad one
docker tag ghcr.io/bloodarea/new-api:<good-tag> new-api-local:local
cd deploy && docker compose up -d
```

Postgres data survives an image swap, but **a new-api downgrade is not always
schema-safe** — `AutoMigrate` adds columns and never removes them. Dump before a
version jump:

```sh
docker exec new-api-postgres pg_dump -U root new-api | gzip > backup-$(date +%Y%m%d).sql.gz
```

Backups contain tokens, password hashes and channel keys — `.gitignore` blocks
`*.sql`, keep it that way.

## License

Upstream new-api is **AGPL-3.0**; the patches here are derivative work under the
same license. This repository contains no upstream source — `src/` is fetched at
build time and stays out of git. Self-hosting for personal use is fine; serving
it to others or redistributing the image brings AGPL source obligations.
