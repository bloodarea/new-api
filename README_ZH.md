# new-api — 定制版

自用的 [new-api](https://github.com/QuantumNous/new-api) 定制版。产品本身是什么、怎么用，以上游 [README](https://github.com/QuantumNous/new-api#readme) 为准；这个分支只写三件事：我们改了什么、怎么构建、怎么部署。

这是私人定制，不打算往上游提 PR。

> English version: [README.md](README.md)

```sh
git clone -b deploy-tooling https://github.com/bloodarea/new-api.git new-api-deploy
```

## 和上游的差异

基线：`v1.0.0-rc.41`（上游 commit `2035a82`），三个补丁按文件名顺序应用。

| 补丁 | 改了什么 |
|---|---|
| `01-channel-site-icon` | 渠道可以带一个网站地址，控制台会去抓该站点的 favicon 并显示在渠道列表里。新增 `GET /api/channel/fetch_icon`（权限 `ChannelOperate`）、渠道设置里的 `SiteURL`/`IconURL`（只存在 settings JSON，不改表结构）、`<link rel="…icon…">` 解析并以 `<origin>/favicon.ico` 兜底、SSRF 防护的 HTTP client，以及抽屉/列表 UI 和一个单元测试。 |
| `02-web-cache-ratelimit` | 两个控制台问题。**缓存**：只有带内容 hash 的 `/static/*` 才给长 `max-age`，`index.html` 和 SPA 兜底页给 `no-store`，避免浏览器拿旧 HTML 去请求上一版构建的 chunk。**限流**：`static.Serve` 提到 `GlobalWebRateLimit` 之前——一次打开控制台会拉几十个 chunk，算进每 IP 配额后正常刷新就 429 / `ChunkLoadError`。 |
| `03-docker-mirror` | 只影响构建过程：把基础镜像和包管理器指到国内源（`docker.m.daocloud.io`、npmmirror、goproxy.cn），并写 `VERSION`。不改变产物——CI 跳过它，改用 `BASE_REGISTRY=` 空值从 docker.io 拉。 |

CI 只打 `01`、`02`；`03` 是给国内机器本机构建用的。

## 分支

三个分支都在这个 fork 里：

| 分支 | 用途 |
|---|---|
| `main` | 上游镜像 + `.github/workflows/build-image.yml`。GitHub 只从默认分支列出 workflow，所以 Actions 页面的「Run workflow」按钮靠它。 |
| `deploy/rc.41` | CI 实际编译的树：上游 tag + `01` + `02` + `VERSION` + `deploy/Dockerfile`。**生成物，别手改。** |
| `deploy-tooling` | 当前这个分支：补丁、构建脚本、compose、文档。 |

## 构建

**GitHub Actions（常规路径）**：Actions → *Build patched image* → Run workflow，参数保持默认。它会用 Bun 1.4.0 编前端、Go 1.26.1 编后端、打包运行时镜像并推送：

```
ghcr.io/bloodarea/new-api:v1.0.0-rc.41
ghcr.io/bloodarea/new-api:latest
```

裸二进制另外作为 artifact 上传（保留 14 天）。不需要配 secret——workflow 用自带的 `GITHUB_TOKEN` 推送；包是 public，运行机免登录就能拉。

**本地构建（备选）**：`./build.sh` 在同一台机器上做同样的事，产出镜像名 `new-api-local:local`，需要 Git、Go、Bun、Docker。工具链查找顺序是 `$GO_BIN_DIR` / `$BUN_BIN_DIR` → `toolchains/` → `PATH`；缓存都落在 `cache/`，不写 `$HOME`。

## 部署

```sh
git clone -b deploy-tooling https://github.com/bloodarea/new-api.git new-api-deploy
cd new-api-deploy

docker pull ghcr.io/bloodarea/new-api:v1.0.0-rc.41
docker tag  ghcr.io/bloodarea/new-api:v1.0.0-rc.41 new-api-local:local

cd deploy
cp ../.env.example ../.env      # 填密钥
docker compose --env-file ../.env up -d
```

- 服务端口 `3000`；Postgres 数据在 `pg_data` volume，上传文件在 `deploy/data`，日志在 `deploy/logs`。
- `SESSION_SECRET` 用来签 session cookie，改了所有人都会被登出。密钥用 `openssl rand -hex 32` 生成。
- compose 给容器设了 `HTTP_PROXY`/`HTTPS_PROXY`，默认指向宿主机 7890 端口；宿主机能直连就把 `.env` 里的 `HTTP_PROXY_URL=` 置空。

## 改东西的流程

```sh
# 1. 改这个分支上的 patches/*.patch
# 2. 重新生成 CI 编译的分支（以及 main 上的 workflow）
FORK_CLONE=~/SourceCodeLibrary/Projects/new-api ./fork-branch.sh
# 3. Actions -> Build patched image -> Run workflow
```

`fork-branch.sh` 会重置它指向的那个克隆里的 `main` 和 `deploy/rc.41`，所以要给一个干净的 fork 克隆。

升级上游：改 `UPSTREAM_TAG`（`build.sh` 和脚本里各一处），把不再适用的补丁 rebase 掉，再重新生成。

回滚：

```sh
docker tag new-api-local:local new-api-local:broken   # 留一份坏的
docker tag ghcr.io/bloodarea/new-api:<好用的 tag> new-api-local:local
cd deploy && docker compose up -d
```

Postgres 数据不受换镜像影响，但 **new-api 降级不一定对表结构安全**——`AutoMigrate` 只加列不删列。跨版本跳之前先 dump：

```sh
docker exec new-api-postgres pg_dump -U root new-api | gzip > backup-$(date +%Y%m%d).sql.gz
```

备份里有 token、密码哈希和渠道密钥——`.gitignore` 挡住了 `*.sql`，别放开。

## 许可证

上游 new-api 是 **AGPL-3.0**，这里的补丁属于衍生作品，同样适用。本仓库不含上游源码——`src/` 在构建时拉取，不进 git。自己用没问题；对外提供服务或分发镜像会触发 AGPL 的源码义务。
