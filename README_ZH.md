# new-api-deploy

自建 [new-api](https://github.com/QuantumNous/new-api) 实例的**部署配置仓库**：我们维护在上游之上的补丁、构建脚本、以及 compose 编排。

> English version: [README.md](README.md)

**本仓库不包含上游源码。** 上游代码在构建时拉取，仓库里只放"怎么把上游变成我们要的样子"——几百 KB，而不是别人 80 MB 的提交历史。

---

## 目录

- [这是什么，不是什么](#这是什么不是什么)
- [仓库结构](#仓库结构)
- [运行需要预构建镜像（重要）](#运行需要预构建镜像重要)
- [编译工具链](#编译工具链)
- [构建](#构建)
- [部署](#部署)
- [对上游的修改](#对上游的修改)
- [升级上游](#升级上游)
- [回滚](#回滚)
- [许可证](#许可证)

---

## 这是什么，不是什么

**是**：一套可复现的构建配方 + 部署编排。克隆下来、装好工具链、跑 `./build.sh`，就能得到与本机正在运行**逐字节相同**的二进制和镜像。

**不是**：new-api 的 fork。上游源码一行都不存。三个补丁加起来 776 行 diff，这就是我们对上游的全部改动。

**也不是**：开箱即用的部署包。**必须先构建镜像才能运行**，见下一节。

---

## 仓库结构

```
new-api-deploy/
├── README.md              英文文档
├── README_ZH.md           本文档
├── build.sh               构建脚本：拉上游 → 打补丁 → 编译 → 打包镜像
├── .env.example           配置模板（复制成 .env 后填密钥）
├── .gitignore             挡住密钥/源码/产物/数据/备份
├── patches/               对上游的改动，按文件名顺序应用
│   ├── 01-channel-site-icon.patch
│   ├── 02-web-cache-ratelimit.patch
│   └── 03-docker-mirror.patch
└── deploy/
    ├── docker-compose.yml  编排：new-api + postgres + redis
    └── Dockerfile          最小运行时镜像（只有二进制和许可证）
```

构建后会出现（均被 `.gitignore` 挡住，不会进 Git）：

```
src/          上游源码 checkout（build.sh 自动 clone）
runtime/      编译产物：二进制 + licenses + Dockerfile
cache/        Go module / 编译 / Bun 缓存
toolchains/   可选：仓库内的工具链（见下节）
```

---

## 运行需要预构建镜像（重要）

**`docker compose up` 之前，`new-api-local:local` 必须已经存在于本机。**

compose 里没有 `build:` 段，镜像也不在任何公共 registry 上：

```yaml
services:
  new-api:
    image: new-api-local:local   # 本地构建，无处可拉
```

如果镜像不存在，`docker compose up` 会尝试去 Docker Hub 拉 `new-api-local`，然后失败（网络不通时表现为长时间卡住后超时）：

```
 Image new-api-local:local Pulling
 Image new-api-local:local Error failed to resolve reference
   "docker.io/library/new-api-local:local": ... i/o timeout
Error response from daemon: failed to resolve reference
   "docker.io/library/new-api-local:local"
```

> 注意报错里出现的是 `docker.io/library/new-api-local` —— compose 把没有 registry 前缀的名字当成了 Docker Hub 上的官方镜像。这个仓库不存在，所以必然失败。

### 镜像的三种来源

| 方式 | 适用场景 | 命令 |
|---|---|---|
| **本机构建** | 有工具链的机器 | `./build.sh` |
| **从别处导入** | 构建机与运行机分离 | `docker save new-api-local:local \| gzip > img.tgz` → 目标机 `gunzip -c img.tgz \| docker load` |
| **私有 registry** | 多机共享 | `docker tag` + `docker push`，然后改 compose 的 `image:` |

> **本项目的实际部署形态**：构建机与运行机分离。构建在一台有 Go/Bun 的机器上做（内存要够），运行机只 `docker load` 或从 registry 拉。运行机不需要 Go、Bun，只要有 Docker。
>
> 这个仓库只描述**怎么构建、怎么运行**，不声明任何一台机器属于哪种环境。哪台机器跑什么，由部署方自己决定并记录在部署方自己的文档里。

### 检查镜像是否存在

```sh
docker image inspect new-api-local:local >/dev/null 2>&1 \
  && echo "镜像存在，可以起" \
  || echo "镜像缺失，先 build.sh 或 docker load"
```

---

## 编译工具链

构建**只需要在一台机器上做一次**。运行机不需要这些。

### 必需的四个工具

| 工具 | 版本要求 | 用途 | 不满足会怎样 |
|---|---|---|---|
| **Git** | ≥ 2.20 | 克隆上游、应用补丁 | `build.sh` 直接退出 |
| **Go** | **与上游 `go.mod` 一致**（rc.41 要求 `go 1.25.1`，实测用 1.26.1 通过） | 编译后端 | 版本过低会因语言特性/标准库 API 报错 |
| **Bun** | **1.4.0**（上游 `Dockerfile` 固定此版本） | 编译前端 | lock 文件格式不兼容会安装失败 |
| **Docker** | ≥ 20.10（需 buildkit） | 打包运行时镜像 | 最后一步失败 |

> **Go 版本为什么必须对**：上游 `go.mod` 的 `go` 指令是硬性下限。用更低版本编译会报 `go.mod requires go >= 1.25.1`。用**更高**版本通常可以，但要留意上游是否用了特定小版本的修复（例如 rc.46 因 CVE-2026-111943 要求 Go 1.26.10，rc.41 无此要求）。

### 工具链放哪里

`build.sh` 按以下顺序查找 `go` 和 `bun`，两者**独立解析**：

1. 环境变量 `GO_BIN_DIR` / `BUN_BIN_DIR` 指向的 `bin` 目录
2. 仓库内的 `toolchains/go/bin` 和 `toolchains/bun/bin`
3. 系统 `PATH` 里已有的 `go` / `bun`

**本机的实际布局是分开的**，所以需要显式指定：

```sh
GO_BIN_DIR=/mnt/software/toolchains/go/bin \
BUN_BIN_DIR=/mnt/software/bun/bin \
./build.sh
```

或者直接把工具链软链进仓库：

```sh
mkdir -p toolchains
ln -s /mnt/software/toolchains/go toolchains/go
ln -s /mnt/software/bun          toolchains/bun
./build.sh
```

如果 `go`、`bun` 本来就在 `PATH` 里（比如用系统包管理器装的），什么都不用设：

```sh
./build.sh
```

### 工具链安装

```sh
# Go —— 从 https://go.dev/dl/ 下载对应版本，解压到任意位置
wget https://go.dev/dl/go1.26.1.linux-amd64.tar.gz
sudo tar -C /opt -xzf go1.26.1.linux-amd64.tar.gz

# Bun
curl -fsSL https://bun.sh/install | bash   # 默认装到 ~/.bun
```

### 缓存位置

`build.sh` 把所有缓存放在仓库内，**不污染 `$HOME`**：

| 变量 | 路径 | 内容 |
|---|---|---|
| `GOPATH` | `cache/gopath` | Go 工作区 |
| `GOMODCACHE` | `cache/gopath/pkg/mod` | 依赖模块 |
| `GOCACHE` | `cache/gocache` | 编译缓存（复用它可大幅缩短重建时间） |
| `BUN_INSTALL_CACHE_DIR` | `cache/bun-cache` | Bun 包缓存 |

`GOPROXY` 默认 `https://goproxy.cn,direct`（国内加速），可用环境变量覆盖：

```sh
GOPROXY=https://proxy.golang.org,direct ./build.sh
```

### 内存要求（重点）

**Go 编译是整个流程里最重的一步**，峰值内存可达数 GB。

| 机器内存 | 建议 |
|---|---|
| ≥ 16 GB | 直接 `./build.sh` |
| 8 GB | `GOMAXPROCS=2 ./build.sh` |
| **≤ 4 GB** | **不要在本机构建**。换机器构建后 `docker save` 传过去 |

> 实测教训：在一台 3.7 GB 内存的 Celeron J1900 上直接构建，`go build` 加 `docker build` 同时进行会触发严重换页，系统失去响应。低配机请务必换机器构建。

---

## 构建

```sh
# 1. 克隆本仓库
git clone http://106.53.27.77:3000/admin/new-api-deploy.git
cd new-api-deploy

# 2. 构建（工具链按上节方式指定）
GO_BIN_DIR=/path/to/go/bin BUN_BIN_DIR=/path/to/bun/bin ./build.sh
```

脚本会依次：

1. **拉上游**：若 `src/` 不存在，`git clone --branch v1.0.0-rc.41 --depth 1`
2. **打补丁**：按文件名顺序应用 `patches/*.patch`
   - 能应用 → 应用
   - 已应用过 → 跳过（可重复执行）
   - **应用失败 → 立即 `exit 1`**，不会静默编出错误版本
3. **编译**：Bun 编前端 → Go 编后端（前端通过 `go:embed` 嵌进二进制）
4. **打包**：`docker build` 出 `new-api-local:local`

构建成功会打印：

```
==> toolchain: go version go1.26.1 linux/amd64
==> toolchain: bun 1.4.0
==> [1/4] cloning ... at v1.0.0-rc.41
==> [2/4] applying patches for v1.0.0-rc.41
    applied 01-channel-site-icon.patch
    applied 02-web-cache-ratelimit.patch
    applied 03-docker-mirror.patch
==> [3/4] frontend build (v1.0.0-rc.41)
    backend build
==> [4/4] runtime image (new-api-local:local)
==> done: new-api-local:local (v1.0.0-rc.41)
```

### 可覆盖的变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `UPSTREAM_TAG` | `v1.0.0-rc.41` | 上游版本，**改了补丁可能失配** |
| `UPSTREAM_URL` | `https://github.com/QuantumNous/new-api.git` | 上游地址 |
| `IMAGE` | `new-api-local:local` | 产出镜像名 |
| `GOPROXY` | `https://goproxy.cn,direct` | Go 模块代理 |
| `GOMAXPROCS` | （Go 自动） | 限制编译并发，低内存机器用 |
| `GO_BIN_DIR` / `BUN_BIN_DIR` | （见上节） | 工具链位置 |

---

## 部署

```sh
cd deploy

# 1. 准备密钥（三份，全部用 openssl 生成）
cp ../.env.example ../.env
vim ../.env        # 填 POSTGRES_PASSWORD / REDIS_PASSWORD / SESSION_SECRET
# 每个都用：openssl rand -hex 32

# 2. 确认镜像存在
docker image inspect new-api-local:local >/dev/null && echo OK

# 3. 起服务
docker compose --env-file ../.env up -d

# 4. 看状态
docker compose --env-file ../.env ps
docker compose --env-file ../.env logs --tail=50 new-api
```

### 端口与数据

| 项 | 位置 |
|---|---|
| 服务端口 | `3000` |
| Postgres 数据 | docker volume `pg_data`（**不在目录里**） |
| 上传文件 | `deploy/data` |
| 日志 | `deploy/logs` |

### 密钥说明

| 变量 | 作用 | 换了会怎样 |
|---|---|---|
| `POSTGRES_PASSWORD` | 数据库密码 | 改了要同步改库，否则连不上 |
| `REDIS_PASSWORD` | Redis 密码 | 同上 |
| `SESSION_SECRET` | 会话签名 | **改了所有用户被登出** |
| `HTTP_PROXY_URL` | 可选，上游请求代理 | 不设则用 `http://host.docker.internal:7890` |

> `.env` 已在 `.gitignore` 里，不会被提交。**不要把它粘进任何聊天或日志。**

### 代理配置

compose 里给 new-api 容器设了 `HTTP_PROXY`/`HTTPS_PROXY`，默认指向宿主机的 `7890` 端口。如果你的环境**不需要代理**，在 `.env` 里设：

```sh
HTTP_PROXY_URL=
```

或者直接删掉 compose 里那四行 proxy 环境变量。

---

## 对上游的修改

基线：**v1.0.0-rc.41**（上游 commit `2035a82`）

三个补丁，共 776 行 diff，涉及 16 个文件。**按顺序应用后，结果与本仓库维护的那份运行代码逐字节一致**（已用 `diff -r` 验证，差异为 0）。

### 补丁 1：`01-channel-site-icon.patch` — 渠道站点与图标

**做了什么**：让每个渠道可以携带一个官网 URL，并在渠道列表里显示该站点的 favicon。

| 层 | 改动 |
|---|---|
| **数据模型** | `relaykit/dto/channel_settings.go` 新增 `SiteURL`、`IconURL` 两个字段（`site_url` / `icon_url`），存入渠道的 settings JSON，不改数据库表结构 |
| **后端接口** | 新增 `GET /api/channel/fetch_icon?url=<站点>`，权限 `ChannelOperate`（与"操作渠道"同级）。返回 `{"icon_url": "..."}` |
| **抓取逻辑** | `controller/channel.go` 里的 `FetchChannelIcon`：接受图片直链或网页 URL；网页则解析 HTML 里的 `<link rel="...icon...">`，取不到就回退到 `<origin>/favicon.ico`；支持 `.ico/.png/.jpg/.gif/.svg/.webp/.avif` |
| **安全** | 使用上游已有的 `service.GetSSRFProtectedHTTPClient()`，防止内网探测 |
| **前端** | 渠道编辑抽屉新增「Channel URL」「Channel Icon」两个输入框 + 「Fetch Icon」按钮；渠道列表列支持显示自定义图标；`api.ts` 新增 `fetchChannelIcon` |
| **i18n** | en / zh-CN / zh-TW 三份语言文件各加 8 个键 |
| **测试** | `controller/channel_icon_test.go`：`TestParseIconFromHTML` 覆盖相对路径、协议相对 URL、多个 `<link>` 的优先级 |

**为什么加**：渠道列表里只有类型图标（OpenAI、Claude 等），分不清同名渠道。有了官网 URL 和 favicon，一眼能认出是哪个上游。

**注意**：`IconURL` 为空时回退到上游默认的类型图标，行为不变。

### 补丁 2：`02-web-cache-ratelimit.patch` — 前端缓存与限流

**做了什么**：修两个导致控制台"白屏 / 加载旧版本 / 报 ChunkLoadError"的问题。

**改动 A —— 缓存策略**（`middleware/cache.go`）

原逻辑：只有 `/` 用 `no-cache`，其他所有路径都给 `max-age=604800`（一周）。问题在于 `index.html` 也被缓存了一周，前端发新版后浏览器还在用旧 HTML，引用的 JS chunk 已经不存在 → 白屏。

改后：

| 路径 | 缓存头 | 理由 |
|---|---|---|
| `/static/*` | `max-age=604800` | 内容哈希命名，内容变了文件名就变，可以放心长缓存 |
| 其他所有路径 | `no-store` + `Pragma: no-cache` | `index.html` 和 SPA 回退页**永不缓存** |

**改动 B —— 限流顺序**（`router/web-router.go`）

原顺序：`GlobalWebRateLimit()` → `Cache()` → `static.Serve()`

问题：控制台一次加载会请求几十个 chunk，全部计入每 IP 的限流预算，正常刷新就可能触发 429，前端表现为 `ChunkLoadError`。

改后：`Cache()` → `static.Serve()` → `GlobalWebRateLimit()`

静态资源先返回（`static.Serve` 命中真实文件就结束请求），只有非静态路径才落到限流器。**限流依然生效**，只是不再把正常的静态资源请求算进去。

### 补丁 3：`03-docker-mirror.patch` — 镜像源与版本号

**做了什么**：把 `Dockerfile` 里的基础镜像指向国内可达的镜像源，并写入 `VERSION` 文件。

| 改动 | 原值 | 改后 |
|---|---|---|
| Bun 构建镜像 | `oven/bun:1.4.0` | `docker.m.daocloud.io/oven/bun:1.4.0` |
| Go 构建镜像 | `golang:1.26.1-alpine` | `docker.m.daocloud.io/library/golang:1.26.1-alpine` |
| 运行时镜像 | `debian:bookworm-slim` | `docker.m.daocloud.io/library/debian:bookworm-slim` |
| Bun registry | （默认） | `BUN_CONFIG_REGISTRY=https://registry.npmmirror.com` |
| Go proxy | （默认） | `GOPROXY=https://goproxy.cn,direct` |
| `VERSION` | 空文件 | `v1.0.0-rc.41` |

**所有 digest 保持不变**，只换了 registry 前缀，所以拉下来的还是同一份镜像。

**如果你在中国境外**：这个补丁可以不用，甚至应该删掉——daocloud 镜像源在境外可能更慢。删掉后从官方源拉，需要能访问 `docker.io` 和 `ghcr.io`。

**`VERSION` 为什么要写**：上游 `VERSION` 在 tag 里是空文件，`build.sh` 用它注入版本号（`-X common.Version=$VER`）。不写的话编译出的二进制版本显示为空。

### 补丁为什么拆成三个

上游升级时，**三个补丁的失配概率差别很大**：

| 补丁 | 上游改动触及它的可能性 | 升级时的工作量 |
|---|---|---|
| 03 镜像源 | 极低（Dockerfile 很少动） | 通常直接可用 |
| 02 缓存/限流 | 中等（`middleware/cache.go`、`router/web-router.go` 偶有改动） | 可能要手工 rebase |
| 01 渠道图标 | 较高（`controller/channel.go` 是活跃文件） | 最可能需要 rebase |

拆开之后，冲突时只需处理失配的那一个，不必推倒重来。

---

## 升级上游

```sh
# 1. 在 src/ 里切到新版本
cd src
git fetch --unshallow          # 首次需要（浅克隆要补全历史）
git checkout v1.0.0-rc.XX

# 2. 回本仓库，逐个试补丁
cd ..
for p in patches/*.patch; do
  git -C src apply --check "$p" 2>/dev/null && echo "OK:   $p" || echo "FAIL: $p"
done
```

对 `FAIL` 的补丁：

```sh
# 在 src/ 里手工应用并解决冲突
cd src
git apply --reject ../patches/01-channel-site-icon.patch
# 编辑冲突文件，删掉 .rej
cd ..

# 重新导出补丁
git -C src diff > /tmp/new.patch
# 核对无误后替换
cp /tmp/new.patch patches/01-channel-site-icon.patch
```

然后：

1. 改 `build.sh` 里的 `UPSTREAM_TAG` 默认值
2. 改本文档和 `README.md` 里的基线版本
3. 重新构建、验证

> **不要只改 `UPSTREAM_TAG` 就构建**。`build.sh` 会在补丁失配时 `exit 1`，但如果你把补丁删了绕过检查，就会编出一个没有我们改动的版本。

---

## 回滚

镜像和数据库是两件事，分开处理。

### 回滚镜像

```sh
# 升级前先留个备份
docker tag new-api-local:local new-api-local:backup-$(date +%Y%m%d)

# 出问题后切回
docker tag new-api-local:backup-20261024 new-api-local:local
cd deploy && docker compose --env-file ../.env up -d
```

### 数据库兼容性（重要）

new-api 用 GORM 的 `AutoMigrate`，它**只加列、从不删列**。这意味着：

| 方向 | 安全性 |
|---|---|
| 升级（旧 → 新） | 安全，自动补列 |
| 降级（新 → 旧） | **通常安全**，旧代码不认识新列但会忽略 |
| 跨过 schema 变更再降级 | **可能出问题**，新版本写入的数据旧版本读不懂 |

**跨版本升级前务必先备份数据库**：

```sh
docker exec new-api-postgres pg_dump -U root new-api | gzip > backup-$(date +%Y%m%d).sql.gz
```

> 备份文件含完整令牌、用户密码哈希、渠道密钥，**不要提交到 Git**（`.gitignore` 已挡 `*.sql`）。

### 服务本身起不来

```sh
# 看日志
cd deploy && docker compose --env-file ../.env logs --tail=100 new-api

# 看健康状态
curl -s http://127.0.0.1:3000/api/status

# 常见原因
# - 镜像不存在        → build.sh 或 docker load
# - 密码改了没同步    → 检查 .env 与数据库是否一致
# - 端口 3000 被占用  → ss -lntp | grep 3000
```

---

## 许可证

上游 new-api 是 **AGPL-3.0**。

- 本仓库的补丁是对 AGPL 代码的修改，构成衍生作品，**同样受 AGPL-3.0 约束**
- 本仓库**不包含**上游源码，`src/` 在构建时拉取且不进 Git
- 自托管自用不涉及分发；**若对外提供服务或再分发镜像，需履行 AGPL 义务**（提供对应源码）

正式对外发版前建议做一次合规审查。
