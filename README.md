# new-api-deploy

Deployment configuration for a self-hosted [new-api](https://github.com/QuantumNous/new-api)
instance: the patches we carry on top of upstream, the build script, and the
compose stack.

> 中文文档见 [README_ZH.md](README_ZH.md)

> **This repo now lives inside the fork of upstream**, as the `deploy-tooling`
> branch — the standalone Gitea repository is retired:
>
> ```sh
> git clone -b deploy-tooling https://github.com/bloodarea/new-api.git new-api-deploy
> ```

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

After a build you also get, all gitignored: `src/` (upstream checkout),
`runtime/` (binary + licenses + Dockerfile), `cache/` (Go and Bun caches),
`toolchains/` (optional, see below).

## Running requires a prebuilt image

**`docker compose up` will fail unless `new-api-local:local` already exists on
the host.** There is no `build:` section and the image is on no registry, so
compose treats the bare name as a Docker Hub official image and tries to pull
it (hanging on a slow network before timing out):

```
 Image new-api-local:local Pulling
 Image new-api-local:local Error failed to resolve reference
   "docker.io/library/new-api-local:local"
```

Three ways to get it there:

| Way | When | Command |
|---|---|---|
| Build locally | machine has the toolchain | `./build.sh` |
| GitHub Actions | build host is short on disk/RAM | run the workflow, then `docker pull` (see below) |
| Import | build host ≠ run host | `docker save new-api-local:local \| gzip > img.tgz`, then `gunzip -c img.tgz \| docker load` |
| Private registry | several hosts | `docker push`, then edit `image:` in the compose file |

The run host needs only Docker — no Go, no Bun.

This repo describes **how to build and how to run**. It makes no claim about
which machine is which environment; that belongs in the deployment operator's
own records, not here.

```sh
docker image inspect new-api-local:local >/dev/null 2>&1 \
  && echo "ready" || echo "missing: run build.sh or docker load"
```

## Toolchain

Building needs four tools, **on one machine only**:

| Tool | Version | Notes |
|---|---|---|
| Git | ≥ 2.20 | clone + apply patches |
| Go | matching upstream `go.mod` (rc.41 says `go 1.25.1`; 1.26.1 verified) | backend |
| Bun | **1.4.0** (pinned in upstream `Dockerfile`) | frontend |
| Docker | ≥ 20.10 with buildkit | image packaging |

Go and Bun are resolved independently, in this order:

1. `$GO_BIN_DIR` / `$BUN_BIN_DIR`
2. `<repo>/toolchains/go/bin` and `<repo>/toolchains/bun/bin`
3. whatever is already on `PATH`

They live apart on the machine this repo was developed on, so:

```sh
GO_BIN_DIR=/mnt/software/toolchains/go/bin \
BUN_BIN_DIR=/mnt/software/bun/bin \
./build.sh
```

Or symlink them in and run plain `./build.sh`:

```sh
mkdir -p toolchains
ln -s /mnt/software/toolchains/go toolchains/go
ln -s /mnt/software/bun          toolchains/bun
```

Caches land in `cache/` (`GOPATH`, `GOMODCACHE`, `GOCACHE`,
`BUN_INSTALL_CACHE_DIR`), never in `$HOME`. `GOPROXY` defaults to
`https://goproxy.cn,direct`; override it with the env var.

**Memory:** the Go build is the heaviest step and can peak at several GB.

| RAM | Advice |
|---|---|
| ≥ 16 GB | just `./build.sh` |
| 8 GB | `GOMAXPROCS=2 ./build.sh` |
| ≤ 4 GB | build elsewhere, ship the image with `docker save` |

A 3.7 GB box running `go build` and `docker build` together thrashed and
became unresponsive — don't build on a low-RAM machine.

## Build

```sh
git clone -b deploy-tooling https://github.com/bloodarea/new-api.git new-api-deploy
cd new-api-deploy
./build.sh
```

It will:

1. `git clone --branch v1.0.0-rc.41 --depth 1` into `./src/` if absent
2. apply every patch in `patches/` — already-applied ones are skipped, so the
   script is re-runnable; a patch that no longer applies makes it **exit 1**
   rather than build the wrong thing
3. build the frontend (Bun) and backend (Go) into `./runtime/`
4. `docker build` the runtime image as `new-api-local:local`

Overridable: `UPSTREAM_TAG`, `UPSTREAM_URL`, `IMAGE`, `GOPROXY`,
`GOMAXPROCS`, `GO_BIN_DIR`, `BUN_BIN_DIR`.

## Building on GitHub Actions

GitHub Actions only runs on GitHub repositories, and this repo lives on our
Gitea — so the CI half lives in the fork of upstream
(`github.com/bloodarea/new-api`), and `./fork-branch.sh` keeps it in sync with
the patches here:

| In the fork | What it is |
|---|---|
| `main` | Upstream mirror + `.github/workflows/build-image.yml`. GitHub only lists workflows from the default branch, so this is what gives you the "Run workflow" button. |
| `deploy/rc.41` | The tree CI builds: upstream `v1.0.0-rc.41` + `patches/01` + `patches/02` + `VERSION` + `deploy/Dockerfile` |

```sh
FORK_CLONE=~/SourceCodeLibrary/Projects/new-api ./fork-branch.sh
```

Re-run that whenever a patch changes; it resets `main` and the deploy branch in
the clone, so point it at a clean clone of the fork. `03-docker-mirror.patch` is
skipped on purpose (its China mirrors only add latency to a runner outside
China), and the runtime image is built with `BASE_REGISTRY=` empty, i.e. plain
docker.io.

Setup, once per fork:

1. Settings → Actions → allow workflows (forks ship with Actions disabled).
2. Actions → **Build patched image** → Run workflow.

No secret is needed: the image goes to GitHub Container Registry and the
workflow pushes it with its own `GITHUB_TOKEN` (`packages: write`). The first
push creates `ghcr.io/bloodarea/new-api` as a **private** package — set it to
public in the package settings if the deploy host should pull without
credentials.

On the run host:

```sh
docker pull ghcr.io/bloodarea/new-api:v1.0.0-rc.41
docker tag  ghcr.io/bloodarea/new-api:v1.0.0-rc.41 new-api-local:local
```

The workflow also uploads the bare binary as a 14-day artifact.

## Deploy

```sh
cd deploy
cp ../.env.example ../.env    # then fill in the three secrets
docker compose --env-file ../.env up -d
```

Generate secrets with `openssl rand -hex 32`. `SESSION_SECRET` signs session
cookies — changing it logs everyone out.

The service listens on `3000`. Data lives in the `pg_data` volume (Postgres)
and `deploy/data` (uploads); logs in `deploy/logs`.

The compose file sets `HTTP_PROXY`/`HTTPS_PROXY` for the new-api container,
defaulting to the host's port 7890. Set `HTTP_PROXY_URL=` empty in `.env` (or
drop those lines) if your host has direct internet.

## Patches

Base: **v1.0.0-rc.41** (upstream `2035a82`). 776 lines of diff across 16 files;
applying all three reproduces the tree we run byte for byte.

| Patch | What it changes |
|---|---|
| `01-channel-site-icon.patch` | Channel site/icon URL feature: a channel can carry a website URL, and the dashboard fetches and shows its favicon. Adds `GET /api/channel/fetch_icon` (permission `ChannelOperate`), `SiteURL`/`IconURL` on channel settings (settings JSON only — no schema change), HTML `<link rel="…icon…">` parsing with `<origin>/favicon.ico` fallback, SSRF-protected via `service.GetSSRFProtectedHTTPClient()`, plus the drawer/column UI, i18n in en/zh/zh-TW, and a unit test. |
| `02-web-cache-ratelimit.patch` | Two fixes for the dashboard loading stale or broken. **Cache**: only content-hashed `/static/*` assets get a long `max-age`; everything else — `index.html` and SPA fallbacks — gets `no-store`, so a browser never keeps HTML pointing at a previous build's chunks. **Rate limit**: `static.Serve` now runs *before* `GlobalWebRateLimit`, because one dashboard load requests dozens of chunks and counting them against the per-IP budget made a normal reload trip 429 / `ChunkLoadError`. |
| `03-docker-mirror.patch` | Points the build stages at a China-reachable mirror (`docker.m.daocloud.io`) and sets `GOPROXY`/`BUN_CONFIG_REGISTRY` to domestic mirrors. Digests are unchanged — same images, different registry prefix. Also writes `VERSION`, which upstream leaves empty at the tag. Cosmetic for anyone outside China — drop it if you build elsewhere. |

They're kept separate on purpose: the mirror patch almost never conflicts, the
cache/limiter one occasionally does, and the channel one touches an active
upstream file, so it's the one most likely to need a rebase.

## Updating upstream

```sh
cd src && git fetch --unshallow && git checkout v1.0.0-rc.XX
```

Then in this repo:

```sh
for p in patches/*.patch; do git -C src apply --check "$p" || echo "CONFLICT: $p"; done
```

Rebase the conflicting patch, bump `UPSTREAM_TAG` in `build.sh` and the base
version in both READMEs, then rebuild.

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
version jump:

```sh
docker exec new-api-postgres pg_dump -U root new-api | gzip > backup-$(date +%Y%m%d).sql.gz
```

Backups contain tokens, password hashes and channel keys — `.gitignore` blocks
`*.sql`, keep it that way.

## License

Upstream new-api is **AGPL-3.0**. The patches here are derivative work and
carry the same license. This repository contains no upstream source; `src/`
is fetched at build time and stays out of git. Self-hosting for your own use
is fine; serving it to others or redistributing the image brings AGPL source
obligations.
