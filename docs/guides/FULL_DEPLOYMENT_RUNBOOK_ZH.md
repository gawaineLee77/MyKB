# MindCreek 完整部署执行手册

> 2026-09-15：本指南仍描述旧 Phase 5 运行组合。R1 已更新空间产品设计，但本地 admin 日常登录、默认空间开户和员工小助手尚待 R2–R6 实现；勿将本指南视为新产品安装验收。见[当前状态](../CURRENT_STATUS.md)。

> 当前生产部署主入口。所有命令从仓库根目录执行；先核对[当前能力与版本](../CURRENT_STATUS.md)。专题配置文档作为补充，历史阶段记录不作为新部署步骤。

本文用于在一台内部 Linux AMD64 服务器上部署当前完整的 MindCreek：MindCreek Phase 5、未修改的 WeKnora v0.8.0、企业 OAuth 2.0、托管默认模型、Personal Notes、Document RAG、原生“问答”知识库、分享/发布/订阅、智能体问答以及只读 MCP。生产环境推荐只开放 HTTPS 80/443，其他容器不得直接暴露。

> 当前新增的“问答”创建入口位于 MindCreek UI 镜像中。若现有 AMD64 归档生成时间早于该功能，必须从最新提交重新构建镜像；仅更新 Git 仓库不会改变已经加载的容器镜像。

## 1. 部署拓扑和路径选择

```text
内部用户 / MCP 客户端
          |
        HTTPS 443
          |
  MindCreek Frontend (Nginx)
          |
  MindCreek Gateway（唯一 API/MCP 入口）
          |
  WeKnora App ---- Docreader
       |             |
   PostgreSQL      Redis
          |
  内部 LLM / Embedding / Rerank / VLM API
```

根据已有数据选择且只选择一种路径：

- **全新部署**：创建新的 Phase 5 卷，适合新服务器。
- **保留数据升级**：沿用现有 Phase 0、Stage 1 或 Phase 5 卷；禁止删除卷。
- **清空重装**：永久删除旧账户、知识库和文件，仅在明确不保留数据时使用第 11 节。

## 2. 服务器准备

最低建议为 4 核 CPU、8 GiB 内存和 50 GiB 磁盘。大量 PDF、500 MiB 文件或多人并发时，建议至少 8 核、16 GiB 内存，并为 Docker 数据目录准备 100 GiB 以上可扩展空间。

安装并检查：

```sh
docker version
docker compose version
git --version
make --version
python3 --version
uname -m
```

要求 Docker Compose v2.24.4 或更高版本，因为部署覆盖文件使用 `!override`。镜像部署不要求服务器安装 Go、Node.js 或 `uv`。服务器时间必须通过 NTP 同步，否则 OAuth 回调和 Token 校验可能失败。

在企业 DNS 中添加 A/AAAA 记录，例如：

```text
mindcreek.example.internal -> 服务器内网 IP
```

生产环境无需 `:18080`。`https://mindcreek.example.internal` 默认使用 443；防火墙只允许受信内网访问 TCP 80/443。

## 3. 准备代码和发布版本

服务器在线时可直接克隆；离线时应传输同一提交的仓库发布包：

```sh
git clone --recurse-submodules https://github.com/gawaineLee77/MyKB.git MindCreek
cd MindCreek
git checkout <已审核的提交或发布标签>
git submodule update --init --recursive
make upstream-status
```

确认 `upstream/weknora` 指向 v0.8.0 的固定提交，并且没有本地修改。生产部署应使用已提交、可追溯的版本，不要直接部署未提交工作区。

## 4. 构建或导入 AMD64 镜像

### 4.1 从最新源代码构建

在能够访问镜像仓库、且已启用 Docker Buildx 的构建机上运行：

```sh
git submodule update --init --recursive
make phase5-check
make phase5-images-pull-amd64
make phase5-images-build-amd64
make phase5-images-list-amd64
make phase5-images-save-amd64
```

`save` 不会覆盖旧归档。如果 `images/archives/` 已有同名 TAR、SHA256 和 manifest，先把这三个旧文件一起移动到带版本号的备份目录，再执行保存。

生成文件：

```text
images/archives/mindcreek-phase5-amd64.tar
images/archives/mindcreek-phase5-amd64.tar.sha256
images/archives/mindcreek-phase5-amd64.tar.manifest.txt
```

归档包含 7 个不同运行时镜像、共 9 个标签：

- `mindcreek-ui:phase5`、`mindcreek-ui:0.6.0`
- `mindcreek-gateway:phase5`、`mindcreek-gateway:0.6.0-phase5`
- `wechatopenai/weknora-app:v0.8.0`
- `wechatopenai/weknora-docreader:v0.8.0`
- `paradedb/paradedb:v0.22.2-pg17`
- `redis:7.0-alpine`
- `python:3.12-alpine`

它不包含运行配置、密码、证书、数据库或用户文档。

### 4.2 在目标服务器导入离线归档

将三件套放入服务器仓库的 `images/archives/`，然后运行：

```sh
(cd images/archives && sha256sum -c mindcreek-phase5-amd64.tar.sha256)
./images/manage.sh load images/archives/mindcreek-phase5-amd64.tar linux/amd64
make phase5-images-list-amd64
```

全部显示 `available linux/amd64` 后再继续。

## 5. 安装服务端 TLS 与企业 CA

从仓库根目录创建受保护目录，并复制 HTTPS 服务证书：

```sh
mkdir -p .local/cert
chmod 700 .local .local/cert
cp /安全来源/fullchain.pem .local/cert/fullchain.pem
cp /安全来源/key.pem .local/cert/key.pem
chmod 644 .local/cert/fullchain.pem
chmod 600 .local/cert/key.pem
```

- `fullchain.pem` 是浏览器访问 MindCreek 时由 Nginx 使用的服务端完整证书链。
- `key.pem` 是对应私钥，必须为普通文件、不能是符号链接，权限必须为 `0600`。

如果企业模型或 OAuth 服务使用内部 CA，还要为 App 和 Gateway 准备合并信任链。它必须同时包含操作系统公共根证书和企业根/中间 CA，不能只放企业 CA：

```sh
cp /etc/ssl/certs/ca-certificates.crt .local/cert/combined-ca.pem
cat /安全来源/corporate-root-and-intermediate.pem >> .local/cert/combined-ca.pem
chmod 644 .local/cert/combined-ca.pem
```

不要把 `SSL_CERT_DIR` 指向普通 PEM 文件。创建仅保存在服务器上的 `.local/compose.corporate-ca.yml`：

```yaml
services:
  gateway:
    environment:
      SSL_CERT_FILE: /etc/ssl/certs/ca-certificates.crt
      HTTPS_PROXY: ${CORPORATE_HTTPS_PROXY:-}
      HTTP_PROXY: ${CORPORATE_HTTP_PROXY:-}
      NO_PROXY: ${CORPORATE_NO_PROXY:-localhost,127.0.0.1,app,gateway,postgres,redis,docreader,mock-embedding}
    volumes:
      - /仓库绝对路径/.local/cert/combined-ca.pem:/etc/ssl/certs/ca-certificates.crt:ro
  app:
    environment:
      SSL_CERT_FILE: /etc/ssl/certs/ca-certificates.crt
      HTTPS_PROXY: ${CORPORATE_HTTPS_PROXY:-}
      HTTP_PROXY: ${CORPORATE_HTTP_PROXY:-}
      NO_PROXY: ${CORPORATE_NO_PROXY:-localhost,127.0.0.1,app,gateway,postgres,redis,docreader,mock-embedding}
    volumes:
      - /仓库绝对路径/.local/cert/combined-ca.pem:/etc/ssl/certs/ca-certificates.crt:ro
```

把 `/仓库绝对路径` 替换为 `pwd` 输出；Compose 绑定挂载应使用绝对路径。只有网络确实要求代理时才在 `.local/mindcreek.env` 中设置 `CORPORATE_HTTPS_PROXY`/`CORPORATE_HTTP_PROXY`。代理地址即使代理 HTTPS 流量，也常以 `http://proxy-host:port` 开头，应以企业网络说明为准。

## 6. 创建生产环境配置

```sh
mkdir -p .local
cp deploy/mindcreek/.env.example .local/mindcreek.env
chmod 600 .local/mindcreek.env
```

生成互不相同的随机值，并立即保存到获批的密码管理系统：

```sh
openssl rand -hex 32
openssl rand -base64 48
openssl rand -hex 16
```

其中 `SYSTEM_AES_KEY` 必须是**正好 32 个 UTF-8 字节**，可使用 `openssl rand -hex 16`。密钥丢失后，数据库中已加密的模型凭据无法恢复。当前生产预检要求数据库、Redis、JWT、AES、LLM、Embedding 和 Rerank 凭据彼此不同。

### 6.1 基础配置

编辑 `.local/mindcreek.env`，至少确认以下值：

```dotenv
WEKNORA_VERSION=v0.8.0
MINDCREEK_PHASE5_UI_IMAGE=mindcreek-ui:phase5
MINDCREEK_PHASE5_GATEWAY_IMAGE=mindcreek-gateway:phase5
MINDCREEK_UI_VERSION=0.6.0
MINDCREEK_VERSION=0.6.0-phase5
MINDCREEK_DEPLOYMENT_ENV=production
TZ=Asia/Shanghai
GIN_MODE=release
AUTO_MIGRATE=true
MAX_FILE_SIZE_MB=500
DOCREADER_GRPC_MAX_FILE_SIZE_MB=

DB_HOST=postgres
DB_PORT=5432
DB_USER=mindcreek
DB_PASSWORD=<独立高强度密码>
DB_NAME=mindcreek
REDIS_ADDR=redis:6379
REDIS_PASSWORD=<另一个独立高强度密码>
REDIS_DB=0
REDIS_PREFIX=mindcreek-phase5:
JWT_SECRET=<独立随机值>
SYSTEM_AES_KEY=<正好32字节>

STORAGE_TYPE=local
LOCAL_STORAGE_BASE_DIR=/data/files
RETRIEVE_DRIVER=postgres
DISABLE_REGISTRATION=true
WEKNORA_TENANT_ENABLE_RBAC=true
WEKNORA_TENANT_ENABLE_CROSS_TENANT_ACCESS=false
WEKNORA_TENANT_AUTO_CREATE_API_KEY=false

MINDCREEK_HTTP_BIND=0.0.0.0
MINDCREEK_HTTP_PORT=80
MINDCREEK_HTTPS_BIND=0.0.0.0
MINDCREEK_HTTPS_PORT=443
MINDCREEK_TLS_CERT_FILE=/仓库绝对路径/.local/cert/fullchain.pem
MINDCREEK_TLS_KEY_FILE=/仓库绝对路径/.local/cert/key.pem
```

`DOCREADER_GRPC_MAX_FILE_SIZE_MB` 留空时继承 500 MiB。Personal Notes 仍只允许 TXT/Markdown；500 MiB 是 Document RAG/问答资料上传上限，不代表建议把单个文件做得这么大。

### 6.2 托管默认模型

如果内部 New API 提供 OpenAI 兼容接口，通常使用 `generic` provider。模型名称必须是服务端实际接受的 model ID，Embedding 维度必须与模型真实输出一致：

```dotenv
MINDCREEK_MANAGED_LLM_NAME=<聊天模型ID>
MINDCREEK_MANAGED_LLM_BASE_URL=https://llm.example.internal/v1
MINDCREEK_MANAGED_LLM_API_KEY=<聊天模型专用Key>
MINDCREEK_MANAGED_LLM_PROVIDER=generic

MINDCREEK_MANAGED_EMBEDDING_NAME=<向量模型ID>
MINDCREEK_MANAGED_EMBEDDING_BASE_URL=https://llm.example.internal/v1
MINDCREEK_MANAGED_EMBEDDING_API_KEY=<Embedding专用Key>
MINDCREEK_MANAGED_EMBEDDING_PROVIDER=generic
MINDCREEK_MANAGED_EMBEDDING_DIMENSION=<实际维度，例如1024>

MINDCREEK_MANAGED_RERANK_NAME=<重排模型ID>
MINDCREEK_MANAGED_RERANK_BASE_URL=https://llm.example.internal/v1
MINDCREEK_MANAGED_RERANK_API_KEY=<Rerank专用Key>
MINDCREEK_MANAGED_RERANK_PROVIDER=generic
MINDCREEK_MANAGED_ALLOW_HTTP=false
MINDCREEK_USER_MODEL_OVERRIDES=false
```

扫描版 PDF 需要 VLM/OCR 时再启用：

```dotenv
MINDCREEK_MANAGED_VLM_ENABLED=true
MINDCREEK_MANAGED_VLM_NAME=<支持图片输入的模型ID>
MINDCREEK_MANAGED_VLM_BASE_URL=https://llm.example.internal/v1
MINDCREEK_MANAGED_VLM_API_KEY=<VLM Key，可与聊天模型共用>
MINDCREEK_MANAGED_VLM_PROVIDER=generic
```

VLM 必须真正支持图片输入。启用后只会影响新解析任务；已有图片分块必须在知识库设置中选择 `builtin-mindcreek-vlm` 并重新解析文档。

### 6.3 企业 OAuth 2.0

本项目由 MindCreek Gateway 适配企业 OAuth 2.0，再向未修改的 WeKnora 提供内部 OIDC broker。不要直接给 WeKnora 配置企业 client secret。针对当前已确认的企业接口，使用 GET 打开 `/authorize`，JSON POST 调用 `/accesstoken`，GET 调用 `/userinfo`：

```dotenv
MINDCREEK_IDENTITY_ENABLED=true
MINDCREEK_IDENTITY_PROTOCOL=oauth2
MINDCREEK_IDENTITY_ALLOW_INSECURE_HTTP=false
MINDCREEK_EXTERNAL_ORIGIN=https://mindcreek.example.internal
MINDCREEK_IDENTITY_PROVIDER_NAME=Corporate account
MINDCREEK_IDENTITY_ISSUER=https://identity.example.internal
MINDCREEK_IDENTITY_AUTHORIZATION_URL=https://identity.example.internal/authorize
MINDCREEK_IDENTITY_TOKEN_URL=https://identity.example.internal/accesstoken
MINDCREEK_IDENTITY_USERINFO_URL=https://identity.example.internal/userinfo
MINDCREEK_IDENTITY_REFRESH_URL=https://identity.example.internal/refreshtoken
MINDCREEK_IDENTITY_CLIENT_ID=<企业分配的client_id>
MINDCREEK_IDENTITY_CLIENT_SECRET=<企业分配的client_secret>
MINDCREEK_IDENTITY_CLIENT_AUTH_METHOD=client_secret_post
MINDCREEK_IDENTITY_AUTHORIZATION_METHOD=GET
MINDCREEK_IDENTITY_AUTHORIZATION_GRANT_TYPE=authorization_code
MINDCREEK_IDENTITY_AUTHORIZATION_DISPLAY=page
MINDCREEK_IDENTITY_TOKEN_REQUEST_FORMAT=json
MINDCREEK_IDENTITY_REDIRECT_URI=https://mindcreek.example.internal
MINDCREEK_IDENTITY_PKCE_ENABLED=false
MINDCREEK_IDENTITY_STATE_REQUIRED=false
MINDCREEK_IDENTITY_USERINFO_TOKEN_TRANSPORT=query
MINDCREEK_IDENTITY_SCOPES=base.profile

MINDCREEK_IDENTITY_USERINFO_DATA_PATH=
MINDCREEK_IDENTITY_SUBJECT_CLAIM=globalUserID
MINDCREEK_IDENTITY_TENANT_CLAIM=tenantId
MINDCREEK_IDENTITY_SUBJECT_TENANT_SCOPED=true
MINDCREEK_IDENTITY_USERNAME_CLAIM=uid
MINDCREEK_IDENTITY_DISPLAY_NAME_CLAIM=uid
MINDCREEK_IDENTITY_UUID_CLAIM=uuid
MINDCREEK_IDENTITY_EMPLOYEE_TYPE_CLAIM=employeeType
MINDCREEK_IDENTITY_EMAIL_CLAIM=
MINDCREEK_IDENTITY_GROUP_CLAIM=
MINDCREEK_IDENTITY_ALLOWED_EMPLOYEE_TYPES=

MINDCREEK_BROKER_CLIENT_ID=mindcreek-weknora
MINDCREEK_BROKER_CLIENT_SECRET=<独立的至少32字符随机值>
MINDCREEK_IDENTITY_TENANT_CREATION_ENABLED=false
```

Gateway 自动在授权请求中加入 `response_type=code`、`scope=base.profile` 和 `display=page`。企业平台登记的 redirect URI 必须精确为外部根地址，例如 `https://mindcreek.example.internal`，企业平台随后返回 `/?code=...`。UserInfo 是平铺 JSON，`MINDCREEK_IDENTITY_USERINFO_DATA_PATH` 必须留空。

Token 请求中的字段和值固定为 `grant_type=authorization_code`。Refresh URL 会被保留用于提供方兼容，但当前 MindCreek 登录会在本地访问会话过期后重新进行企业认证；企业 access/refresh token 不会发送给浏览器或 WeKnora。

如果企业 IdP 的正式文档明确要求表单 POST，才把 `MINDCREEK_IDENTITY_AUTHORIZATION_METHOD` 改为 `POST`。若以后 IdP 支持 state/PKCE，应再将对应开关调整为 `true` 并完成兼容测试。

### 6.4 可选代理

仅在 App/Gateway 无法直接访问企业 HTTPS 服务时加入：

```dotenv
CORPORATE_HTTPS_PROXY=http://proxy.example.internal:8080
CORPORATE_HTTP_PROXY=http://proxy.example.internal:8080
CORPORATE_NO_PROXY=localhost,127.0.0.1,app,gateway,postgres,redis,docreader,mock-embedding
```

不要将代理账号密码提交到 Git。若不用代理，保持这些变量为空。

## 7. 配置预检

生产启动前执行：

```sh
chmod 600 .local/mindcreek.env .local/cert/key.pem
python3 scripts/phase5-secret-check.py --env-file .local/mindcreek.env
make phase5-models-render
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" config --quiet
```

预期看到“Rendered 3 managed model declarations”或启用 VLM 后的 4 个声明，以及 production secrets verified。渲染文件只保存环境变量引用，不保存 API Key。

在 Linux 上若仓库由 root 管理，必须确认容器中的 `appuser` 能读取该只读文件，否则 App 会报告 `builtin_models.yaml: permission denied`：

```sh
docker run --rm --entrypoint id wechatopenai/weknora-app:v0.8.0 appuser
ls -l .local/phase5/builtin_models.yaml
```

当前镜像中的 `appuser` 通常是 UID/GID 1000。root 部署可在首次渲染后执行 `chown 1000:1000 .local/phase5/builtin_models.yaml`；后续由 root 重新渲染时会保留文件所有者。不要放宽 `.local/mindcreek.env` 或 TLS 私钥权限。

没有企业私有 CA 时，省略所有 `-f "$PWD/.local/compose.corporate-ca.yml"` 参数。每次 `config`、`up`、`ps` 和 `logs` 都应使用相同的覆盖文件集合。

## 8. 首次启动

### 8.1 全新生产部署

不要同时叠加 `.local/lan.override.yml`：生产 profile 已经发布 80/443，LAN override 会把拓扑重新改回测试用 HTTP 端口。

```sh
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" up -d --remove-orphans
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" ps
```

首次启动会自动创建数据库并执行 WeKnora 和 MindCreek 迁移，通常需要 1～3 分钟。观察关键服务：

```sh
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" logs --tail=200 postgres redis docreader app gateway frontend
```

不要因为 App 在 PostgreSQL 初始化阶段短暂出现 `connection refused` 就删除卷。先确认 PostgreSQL 变为 healthy；如果 App 没有自动恢复，再执行：

```sh
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" up -d app gateway frontend
```

### 8.2 主机重启后的自动恢复

Compose 服务已有 restart policy，但建议由 systemd 在 Docker 启动后重新执行完整 Compose 命令，确保一次性辅助服务也被拉起。创建 `/etc/systemd/system/mindcreek.service`：

```ini
[Unit]
Description=MindCreek Phase 5
Requires=docker.service
After=docker.service network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/仓库绝对路径
ExecStart=/仓库绝对路径/scripts/phase5-production-compose.sh -f /仓库绝对路径/.local/compose.corporate-ca.yml up -d
ExecStop=/仓库绝对路径/scripts/phase5-production-compose.sh -f /仓库绝对路径/.local/compose.corporate-ca.yml stop -t 60
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target
```

替换绝对路径后执行：

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now mindcreek.service
sudo systemctl status mindcreek.service
```

## 9. 上线验证

### 9.1 容器、端口和 HTTPS

```sh
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" ps
docker inspect MindCreek-postgres --format '{{.State.Health.Status}}'
docker inspect MindCreek-docreader --format '{{.State.Health.Status}}'
docker inspect MindCreek-app --format '{{.State.Health.Status}}'
docker inspect MindCreek-gateway --format '{{.State.Health.Status}}'
docker port MindCreek-app
docker port MindCreek-gateway
docker port MindCreek-postgres
docker port MindCreek-redis
curl -fsS https://mindcreek.example.internal/edge-health
```

四个带 healthcheck 的容器应为 `healthy`；四个内部服务的 `docker port` 应无输出；`edge-health` 应返回 `ok`。Frontend 是唯一应显示 80/443 映射的容器。

`make phase5-runtime-check` 默认检查开发端口 18080，因此不要把它作为生产 HTTPS 健康检查。生产环境使用本节命令以及只读观测探针：

```sh
make phase5-observability-probe
SSL_CERT_FILE="$PWD/.local/cert/combined-ca.pem" \
  MINDCREEK_PROBE_URL=https://mindcreek.example.internal \
  make phase5-gate-b-probe
```

Gate B probe 只检查身份配置和关闭注册，不执行真实企业登录。不要在正式库运行 `phase5-gate-a-probe` 或 `phase5-pilot-probe`，它们会创建合成用户、知识库、文档和会话。

### 9.2 浏览器与功能验收

用一个试点员工账户逐项验收：

1. 打开外部域名，证书可信，HTTP 自动跳转 HTTPS。
2. 登录页只提供企业账户入口，不显示本地注册；企业登录完成后首次自动创建 MindCreek 用户和个人空间。
3. 设置 → 模型配置中可看到只读的 Chat、Embedding、Rerank，以及启用时的 Vision/OCR；逐个执行连接测试。
4. 创建 Personal Notes，新增并保存 Markdown 笔记。
5. 创建 Document RAG，上传文件并等待解析完成；用引用来源回答一个问题。
6. 创建原生“问答”知识库，导入 FAQ 表格；确认智能体能够检索 FAQ，FAQ 问题可用于问题推荐。
7. 创建智能体，分别验证无知识库聊天和指定知识库问答。
8. 验证私有、定向分享、组织公开、发布、订阅和取消订阅；撤销权限后下一次查询立即失败。
9. 如需 MCP，使用受控 Bearer Token 或范围化 API Key 访问 `POST https://mindcreek.example.internal/mcp`；不得暴露 Gateway 容器端口或启用匿名 MCP。

## 10. 保留数据升级

### 10.1 已经使用 Phase 5 卷

先备份，再停止旧栈；`down` 后面绝对不要加 `-v`：

```sh
make phase5-backup
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" up -d
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" down
```

保存备份脚本输出的目录，并复制到加密、受控的异机存储。随后更新已审核代码、子模块和镜像：

```sh
git pull --ff-only
git submodule update --init --recursive
(cd images/archives && sha256sum -c mindcreek-phase5-amd64.tar.sha256)
./images/manage.sh load images/archives/mindcreek-phase5-amd64.tar linux/amd64
make phase5-images-list-amd64
python3 scripts/phase5-secret-check.py --env-file .local/mindcreek.env
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" config --quiet
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" up -d --remove-orphans
```

重新执行第 9 节。`up -d` 会保留当前 Phase 5 named volumes，并在 App 启动时自动执行向前迁移。

### 10.2 从 Phase 0 或 Stage 1 卷升级

先按照旧版本文档完成数据库、上传文件和环境配置备份，然后停止旧项目。不要让旧版和新版同时连接同一 PostgreSQL 卷：

```sh
make phase0-down
# 如果旧服务由 Stage 1 启动，则改用：make stage1-down
docker volume ls --format '{{.Name}}' | grep -E 'postgres-data|data-files|docreader-tmp'
```

将查到的真实卷名写入 `.local/mindcreek.env`：

```dotenv
MINDCREEK_PHASE0_POSTGRES_VOLUME=mindcreek-stage1_postgres-data
MINDCREEK_PHASE0_FILES_VOLUME=mindcreek-stage1_data-files
MINDCREEK_PHASE0_DOCREADER_VOLUME=mindcreek-stage1_docreader-tmp
```

示例中的 `mindcreek-stage1_*` 必须替换为服务器实际卷名。然后始终叠加外部卷覆盖：

```sh
./scripts/phase5-production-compose.sh \
  -f "$PWD/deploy/phase4/compose.phase0-volumes.yml" \
  -f "$PWD/.local/compose.corporate-ca.yml" \
  config --quiet
./scripts/phase5-production-compose.sh \
  -f "$PWD/deploy/phase4/compose.phase0-volumes.yml" \
  -f "$PWD/.local/compose.corporate-ca.yml" \
  up -d
```

以后所有生产操作都必须继续带这两个 `-f` 参数；遗漏 external-volume 覆盖会创建一套看似“数据丢失”的空卷。WeKnora v0.8.0 首次启动会把上游数据库迁移推进到 000090。若要从 v0.8.0 回退到 v0.7.2，必须恢复升级前数据库备份。

## 11. 清空旧环境后全新安装

以下操作会永久删除已知 MyKB/MindCreek Compose 项目的容器、网络和 named volumes，包括账户、知识库、上传文件、索引和审计记录。先查看目标：

```sh
./scripts/phase5-server-reset.sh --list
```

仅在确认不保留任何数据后执行：

```sh
./scripts/phase5-server-reset.sh --confirm-destroy-all-mindcreek-data
./scripts/phase5-server-reset.sh --list
```

该脚本保留 Docker 镜像、离线归档、仓库文件和 `.local` 配置。清理完成后按照第 8.1 节重新启动，不要使用 external-volume 覆盖。

## 12. 备份、恢复与回滚

### 12.1 一致性备份

```sh
make phase5-backup
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" up -d
```

备份包含 PostgreSQL custom dump、`data-files`、迁移清单、产品配置和校验和，不包含 `.local/mindcreek.env`、模型 Key 或 TLS 私钥。密钥和 `SYSTEM_AES_KEY` 必须通过独立的企业 Secret 管理系统备份。

备份脚本会短暂停止写入服务；完成后再次执行相同的 production `up -d`，确保 TLS、企业 CA 和网络覆盖仍按生产配置生效。至少每日备份，并在每次升级前额外保留一份。

### 12.2 恢复

恢复会覆盖当前数据库和文件卷，必须先隔离用户流量：

```sh
./scripts/phase5-restore.sh --confirm-replace-current-data /受保护的备份目录
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" up -d
```

恢复完成后重新执行第 9 节，并抽查笔记、RAG 文档、FAQ、引用、订阅和 MCP。恢复脚本不会恢复 Secret；必须提供与备份时相同的 `SYSTEM_AES_KEY` 和其他运行凭据。

### 12.3 回滚原则

- 保留上一个可用镜像归档、对应 Git 提交和升级前备份，直到观察期结束。
- 仅回滚 MindCreek UI/Gateway、且 WeKnora 仍为 v0.8.0 时，可加载旧 Phase 5 镜像并重新 `up -d`。
- 从 WeKnora v0.8.0 回退到 v0.7.2 时，必须恢复升级前数据库，不得手工修改迁移版本。
- 停止服务可使用 `down`；任何正常升级或回滚都不得使用 `down -v`。

## 13. 日常运维

查看状态和日志：

```sh
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" ps
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" logs --tail=200 app gateway docreader
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" logs -f gateway app
make phase5-observability-probe
docker stats --no-stream
docker system df
```

安全要求：

- `.local/mindcreek.env` 和 TLS 私钥保持 `0600`，不得提交 Git。
- 不在日志、工单和截图中记录 client secret、authorization code、access token、refresh token、文档正文或完整提示词。
- 保持 `LLM_DEBUG_LOG=false`；诊断结束后撤销临时凭据。
- 只开放 Frontend 80/443；Gateway、App、PostgreSQL、Redis、Docreader 不开放主机端口。
- 定期轮换数据库、Redis、JWT、OAuth 和模型凭据。更换 `SYSTEM_AES_KEY` 前必须执行正式的凭据重加密流程，不能直接替换。
- Docker 日志应由主机设置容量和保留周期，备份盘剩余空间低于 1 GiB 时立即告警。

## 14. 常见故障定位

### App 报 PostgreSQL `connection refused`

先检查数据库，不要删除卷：

```sh
docker inspect MindCreek-postgres --format '{{json .State.Health}}'
docker logs --tail=200 MindCreek-postgres
docker exec MindCreek-postgres pg_isready
```

确认 `DB_HOST=postgres`，并且所有服务来自同一个 `mindcreek-phase5` Compose 项目。数据库 healthy 后重新 `up -d app gateway frontend`。如果从旧版本升级，确认 external-volume 覆盖和三个真实卷名没有遗漏。

### 页面显示“企业单点登录尚未由管理员配置”

```sh
curl -fsS https://mindcreek.example.internal/api/v1/mindcreek/oidc/status
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" logs --tail=200 gateway app
```

确认 `MINDCREEK_IDENTITY_ENABLED=true`、外部 origin 为精确 HTTPS 根地址、企业 client secret 至少 16 字符、broker secret 至少 32 字符。修改环境文件后必须重新创建 Gateway、App 和 Frontend：

```sh
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" up -d --force-recreate gateway app frontend
```

### `/authorize` 返回 405

当前企业接口应配置 `MINDCREEK_IDENTITY_AUTHORIZATION_METHOD=GET`。Gateway 将浏览器导航到带 `client_id`、`response_type=code`、`redirect_uri`、`scope` 和 `display` 的查询 URL。只有企业接口明确要求浏览器表单 POST 时才改为 `POST`。

### Token 阶段 `request_failed`、`status=0` 或超时

这通常不是 code 解析错误，而是容器的 DNS、路由、代理或证书链问题。先从 App 容器只测试 TLS/网络，不携带任何凭据：

```sh
docker exec MindCreek-app curl -Iv https://identity.example.internal/accesstoken
docker inspect MindCreek-app --format '{{json .Mounts}}'
docker inspect MindCreek-gateway --format '{{json .Mounts}}'
```

GET 测试收到 400/405 也表示网络和 TLS 已经到达服务；`self-signed certificate in certificate chain` 表示合并 CA 不完整或没有挂载到正确路径。修复后必须 recreate App/Gateway，不能只在运行中的容器里 `docker cp`，因为容器重建后会丢失。

### `builtin_models.yaml: permission denied`

生成文件应存在于 `.local/phase5/builtin_models.yaml`，App 必须能读取。按照第 7 节检查 UID 和所有者；该文件本身不含 API Key，只有环境变量引用。不要把 `.local/mindcreek.env` 改成可公开读取。

### 默认模型可见但不能聊天或连接测试失败

确认模型名称是 API 实际 model ID，Base URL 包含供应商要求的 `/v1`，Embedding 维度正确，App 已信任企业 CA。然后执行：

```sh
make phase5-models-render
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" up -d --force-recreate app gateway frontend
./scripts/phase5-production-compose.sh -f "$PWD/.local/compose.corporate-ca.yml" logs --tail=200 app gateway
```

智能体还必须选择有效 Chat 模型；无 RAG 对话需把知识库选择模式设为 `none`。

### 上传仍是 50/200 MiB 或返回 413

确认 `.local/mindcreek.env` 中 `MAX_FILE_SIZE_MB=500`，并让 `DOCREADER_GRPC_MAX_FILE_SIZE_MB` 留空或同为 500。重建/加载最新 UI 和 Gateway 镜像后，recreate `frontend gateway app docreader`。浏览器、Nginx、Gateway、App 和 Docreader 任一层仍使用旧值都会阻止上传。

### 找不到知识库、资源 404 或没有“问答”创建入口

首先确认已加载当前归档并实际重建了容器，而不是仍使用旧 image ID：

```sh
docker inspect MindCreek-frontend --format '{{.Config.Image}} {{.Image}}'
docker inspect MindCreek-gateway --format '{{.Config.Image}} {{.Image}}'
docker image inspect mindcreek-ui:phase5 --format '{{.Id}}'
docker image inspect mindcreek-gateway:phase5 --format '{{.Id}}'
```

容器 image ID 应与当前标签一致。否则重新加载最新 AMD64 归档并执行 `up -d --force-recreate frontend gateway`。若是从旧卷升级，还要确认 Gateway 迁移成功，并检查当前用户对知识库的 owner/share/subscription 授权。

## 15. 上线完成清单

只有全部满足后才向内部用户开放：

- [ ] Git 提交、WeKnora 子模块提交和 AMD64 归档 SHA256 已记录。
- [ ] 生产环境文件与私钥权限正确，Secret 已保存到批准的系统。
- [ ] HTTPS 证书可信，企业 CA/代理在 App 和 Gateway 内有效。
- [ ] 只有 Frontend 暴露 80/443，所有内部服务无主机端口。
- [ ] PostgreSQL、Docreader、App、Gateway 均 healthy。
- [ ] 企业 OAuth 登录、首次用户创建、关闭本地注册和停用用户拒绝均已验证。
- [ ] Chat、Embedding、Rerank、可选 VLM 连接测试成功。
- [ ] Personal Notes、Document RAG、问答 FAQ、智能体及引用链路通过。
- [ ] 分享、组织公开、发布、订阅、撤销和只读 MCP 权限通过。
- [ ] 完整备份已复制到异机受控存储，恢复负责人和回滚窗口明确。

## 16. 相关文档

- [总体设计](../OVERALL_DESIGN_ZH.md)
- [Phase 5 运维说明](PHASE5_OPERATIONS.md)
- [企业身份提供方配置](PHASE5_IDENTITY_PROVIDER.md)
- [备份与恢复](PHASE5_BACKUP_RECOVERY.md)
- [WeKnora v0.8.0 升级记录](../upgrades/WEKNORA_V0.8.0_UPGRADE.md)
- [全新服务器安装与清理](PHASE5_FRESH_SERVER_INSTALL.md)
