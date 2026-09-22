# MindCreek R4：AMD64 企业内网部署与调测

适用版本：`r4-20260916-118b8af3`；WeKnora 固定为 `v0.8.0`。

可选前端补丁：通过[企业文档链接配置](R4_ENTERPRISE_LINKS_ZH.md)隐藏上游帮助/GitHub 等入口，并在将来配置内部文档地址。需要先升级一次前端，原包不会自动具备此开关。

本手册面向**全新 Linux AMD64 安装**。安装包包含 R4 原生界面、产品网关和全部必需运行镜像；目标机器无需 Go、Node、源码仓库或在线拉取镜像。模型服务与企业 OAuth 服务由企业提供，不包含模型权重。

本地验收使用 ARM64 主机上的 AMD64 容器模拟执行、隔离数据库、合成员工和模型。包内 `evidence/amd64-smoke.json` 记录实际结果；企业身份、真实模型效果、物理 x86 服务器性能和生产运维仍需按本文验收。此包用于企业部署调测，不代表 R6 生产验收已经完成。

> **已知阻碍：关键词/混合检索尚未通过 AMD64 验收。** 本机模拟运行 AMD64 ParadeDB 时，最小 BM25 查询会触发数据库进程异常退出；同版本 ARM64 查询正常。尚未确定是否仅与模拟执行有关，不能据此断言物理 AMD64 一定正常。包保留原生检索默认配置；向量模式问答单独验证，不替代混合检索验收。导入企业数据前，必须先在目标 Linux AMD64 服务器运行第 3 节的 `check-database`，失败时停止安装并检查报告。详见包内 `evidence/bm25-diagnostic.json`。

可选图谱能力使用[离线图谱补充包与指南](R4_KNOWLEDGE_GRAPH_ZH.md)，在现有安装上增加 Neo4j/APOC 并更新网关；无需重装账户。

## 1. 包内容与部署范围

| 镜像 | 用途 | 日常运行 |
|---|---|---|
| `mindcreek-ui:r4-20260916-118b8af3` | R4 原生页面与产品登录/成员管理适配 | 是 |
| `mindcreek-gateway:r4-20260916-118b8af3` | OAuth、admin、默认空间、原生权限接入、MCP | 是 |
| `wechatopenai/weknora-app:v0.8.0` | 原生知识库、Agent、问答与成员权限 | 是 |
| `wechatopenai/weknora-docreader:v0.8.0` | 文档解析 | 是 |
| `paradedb/paradedb:v0.22.2-pg17` | PostgreSQL、向量与关键词索引、产品数据 | 是 |
| `redis:7.0-alpine` | 队列、流式状态，开启持久化 | 是 |
| `python:3.12-alpine` | 包内运维工具与备份数据卷 | 按需 |
| `nginx:1.30.3-alpine` | 可选 HTTPS 入口 | 按需 |

所有镜像为 `linux/amd64`。精确镜像 ID、注册表摘要、源码指纹与前端构建指纹见 `RELEASE.json`。这是当前已验收工作树的快照，包含尚未提交的产品改动；不能只凭 Git HEAD 重建它。

支持：原生知识库/FAQ/手工文档/Agent、符合原生条件的 Wiki、四角色、默认模型、独立 admin 与企业员工登录、MCP 四工具。默认关闭图谱、Docker 沙箱和外部观测服务。图片/扫描件的语义解析还需正确配置 VLM；额外 OCR 服务不在此包。R5 员工网页小助手尚未交付。旧发布订阅、个人笔记接口返回 410。

```text
mindcreek-r4-20260916-118b8af3-amd64/
├── images.tar                    # 八个镜像；docker save 格式
├── RELEASE.json                  # 镜像与构建来源
├── SHA256SUMS                    # 包内文件校验和
├── compose.json                  # 独立企业运行组合，不含模拟服务
├── bin/mindcreek                 # Python 部署工具
├── bin/check-database            # 独立目标机 BM25 检查，不接触业务卷
├── enterprise.env.example        # 无真实凭证的模板
├── config/                       # 路由、原生配置、公开 CA、TLS 模板
├── scripts/                      # 托管模型配置生成器
├── deploy/phase5/                # 复用的模型声明模板，不是旧业务组合
├── evidence/amd64-smoke.json      # 本次镜像包验收
├── licenses/                     # 许可证与第三方说明
└── DEPLOYMENT_ZH.md               # 本文
```

## 2. 服务器与网络准备

### 2.1 系统和资源

- Linux x86_64/AMD64；`uname -m` 应输出 `x86_64`。不要在 ARM 生产服务器上把模拟执行当作原生部署。
- Docker Engine 与 Compose 插件。本次工具验证版本为 Docker 29.3、Compose 5.1；建议企业使用已批准的同系列版本。脚本使用 `image inspect --platform`、Compose `up --wait` 和 `pull_policy`，旧版 Docker/Compose 需先升级验证。
- Python 3.10 或以上，用于宿主机部署脚本；无第三方 Python 包依赖。
- 小规模联调建议从 **8 vCPU、16–32 GB 内存、100 GB 可用磁盘**开始，另预留文档、索引、备份与旧镜像空间。这是初始规划值，需通过实际并发和数据量测量调整。远程模型模式无需本机 GPU。
- 服务器时间同步；Docker 数据根目录使用持久磁盘。离线机器的 Docker/Python 系统安装包由企业 OS 软件仓库提供，本包不含这些 OS 安装程序。

核对命令：

```sh
uname -m
docker version
docker compose version
python3 --version
df -h
```

Docker 安装步骤以企业 OS 基线和 [Docker 官方安装文档](https://docs.docker.com/engine/install/)为准。Docker 操作账户应由企业管理员配置；不要为方便调测开放未认证 Docker TCP API。

### 2.2 访问方向和端口

```text
员工浏览器 → 企业 HTTPS 入口:443 → frontend:80 → gateway:8080 → app:8080
                                                   ├→ 企业 OAuth HTTPS
                                                   └→ PostgreSQL
app → PostgreSQL / Redis / DocReader / 企业模型 HTTPS
```

- 默认宿主只发布 `127.0.0.1:18080`，供同机企业 HTTPS 代理访问。
- 如果 HTTPS 代理在另一台机器，将 `FRONTEND_BIND_IP` 设置为应用机内网 IP，防火墙只允许代理机器访问 `FRONTEND_PORT`。
- 可选包内 TLS 只发布 HTTPS 443。PostgreSQL、Redis、DocReader、app、gateway 不发布宿主端口。
- 浏览器必须能访问主站域名与 OAuth 授权地址；gateway 容器必须能解析并访问企业 Token/UserInfo 地址；app 必须能访问模型服务。
- Docker 中的 `localhost` 指容器自己。模型在宿主机时使用明确的内网地址，并配置相应防火墙及精确白名单。
- 不挂载旧 Phase 0–5 或 R2/R3 测试卷；本包采用新的 Compose project 与命名卷。

## 3. 传输、校验与导入镜像

将同名 `.tar.gz` 和 `.tar.gz.sha256` 两个文件传入服务器。

```sh
sha256sum -c mindcreek-r4-20260916-118b8af3-amd64.tar.gz.sha256
# 由运维授予当前部署账户写入这两个目录的权限。
sudo install -d -m 0750 -o "$(id -u)" -g "$(id -g)" /opt/mindcreek /srv/mindcreek
tar -xzf mindcreek-r4-20260916-118b8af3-amd64.tar.gz -C /opt/mindcreek
cd /opt/mindcreek/mindcreek-r4-20260916-118b8af3-amd64
python3 bin/mindcreek load
python3 bin/check-database --output /srv/mindcreek/database-check.json
```

`load` 先验证 `SHA256SUMS`，再导入 `images.tar`，最后逐项比对八个镜像的 AMD64 配置 ID。原始 Docker 命令见 [image save](https://docs.docker.com/reference/cli/docker/image/save/)与 [image load](https://docs.docker.com/reference/cli/docker/image/load/)文档。包内镜像不会自动在线拉取。

只有架构正确、镜像 ID 全部匹配才继续。不要运行全局 `docker system prune`，也不要为了导入镜像删除其他项目的容器和卷。

`check-database` 使用本包数据库镜像、新建随机名称容器和内存临时数据目录，关闭网络、不发布端口，执行建索引与 BM25 查询后移除该测试容器。**必须得到 `passed` 才继续安装业务实例**。它不验证真实模型或企业身份。若出现数据库崩溃，保留报告并停止；不要靠关闭所有知识库的关键词索引来把该项标成通过。

## 4. 初始化独立实例目录

以后每次维护都使用相同的以下变量和同一部署账户。运行状态在镜像包目录之外，升级时不随包替换。

```sh
export MINDCREEK_STATE_DIR=/srv/mindcreek/r4-instance
export MINDCREEK_PROJECT=mindcreek-enterprise-r4
umask 077
python3 bin/mindcreek init
```

工具交互输入本地 admin 初始密码（12–32 位，包含字母和数字，二次确认），自动生成数据库密码、JWT 密钥、32 字节 AES 密钥、Redis 密码、内部身份桥接密钥。上游限制密码最大 32 位，不能使用更长字符串。密码不写在命令参数中；不存在通用默认密码。

实例目录结构：

```text
r4-instance/
├── enterprise.env          # 企业配置和服务凭证，0600
├── secrets/
│   ├── admin-password      # 本地管理员密码，0600
│   ├── member-key          # 初始化默认空间时生成，0600
│   └── recovery-owner-bearer  # 仅显式修复时临时放置
├── runtime/                # 自动生成模型声明与合并 CA
├── ca/                     # 企业 CA PEM，*.crt，只放证书
├── tls/                    # 可选入口的 fullchain.pem 与 privkey.pem
└── backup/                 # 备份输出，仍需复制到异机受控存储
```

`init` 拒绝覆盖已有实例。不要重新生成 AES/JWT/数据库秘密来“修复”旧实例；AES 丢失会使已加密配置无法解密。实例目录 0700，密钥及 `enterprise.env` 为 0600。`MINDCREEK_UID/GID` 必须是秘密文件所有者的数值 UID/GID，容器通过该身份读取管理秘密。

## 5. 配置企业身份、模型与证书

编辑 `$MINDCREEK_STATE_DIR/enterprise.env`，替换所有 `REPLACE_*`。文件按字面 `KEY=value` 或 `KEY='value'` 读取，不执行 Shell 展开；不支持行尾注释/多行值。不要 `source` 这个文件，不把内容贴进调测工单。

### 5.1 域名和企业 OAuth2

沿用旧 Phase 5 企业身份服务时，先核对[协议配置差异及诊断说明](R4_IDENTITY_DIAGNOSTICS_ZH.md)。当前模板的授权方法、请求格式、state/PKCE 和用户字段并非旧企业协议的默认值。

原 `r4-20260916-118b8af3` 镜像对企业 Client Secret 有 16 字符限制。若企业签发的值较短，先应用[网关长度校验补丁](R4_OAUTH_CLIENT_SECRET_HOTFIX_ZH.md)；修复后仅要求非空，不能自行补长企业凭证。

| 配置 | 填写内容 |
|---|---|
| `MINDCREEK_EXTERNAL_ORIGIN` | 最终 HTTPS 主站，例如 `https://mindcreek.company.internal`，不含子路径 |
| `MINDCREEK_INSTALL_ADMIN_EMAIL` | 独立本地 admin 的唯一邮箱标识；不要与员工企业身份共用 |
| `MINDCREEK_IDENTITY_ISSUER` | 身份服务发行者标识/根地址 |
| `MINDCREEK_IDENTITY_AUTHORIZATION_URL` | 企业授权端点 |
| `MINDCREEK_IDENTITY_TOKEN_URL` | 企业 Token 端点 |
| `MINDCREEK_IDENTITY_USERINFO_URL` | 员工信息端点 |
| `MINDCREEK_IDENTITY_CLIENT_ID/CLIENT_SECRET` | 企业申请的 OAuth 应用凭证 |
| `MINDCREEK_IDENTITY_SUBJECT_CLAIM` | 稳定且唯一的员工主体字段，模板为 `sub` |
| `...USERNAME_CLAIM / DISPLAY_NAME_CLAIM / EMAIL_CLAIM` | 与实际员工信息 JSON 字段一致 |
| `MINDCREEK_IDENTITY_SCOPES` | 企业接口实际要求的 scope，不能机械照搬模板 |

支持直接回调的企业 OAuth 应用可登记：

```text
https://你的主站域名/api/v1/mindcreek/oidc/callback
```

使用上述回调路径时，在实例 `enterprise.env` 中同时明确设置 `MINDCREEK_IDENTITY_REDIRECT_URI=https://你的主站域名/api/v1/mindcreek/oidc/callback`。当前 OAuth2 适配器为兼容已有企业协议，未配置此项时使用主站根地址；仅设置 `MINDCREEK_EXTERNAL_ORIGIN` 不会自动选择上述完整回调路径。

内部 WeKnora 桥接回调为 `/api/v1/auth/oidc/callback`，由产品使用。不要把内部 gateway 容器地址登记成企业应用的公网回调。

模板按标准 OAuth2 `GET authorize`、`form token`、`client_secret_post` 配置。企业接口若使用 JSON、POST 授权、嵌套 UserInfo 或不同字段，要依据实际协议修改对应 `MINDCREEK_IDENTITY_*`，并先验证匿名授权地址、state、PKCE、唯一主体、邮箱映射。不要用改管理员账号的方式绕过身份映射问题。OIDC 模式还需核对 Discovery、Issuer、`openid` scope 与 JWKS；该包默认选 OAuth2。

支持 state/PKCE 的提供者保留 `STATE_REQUIRED=true` 与 `PKCE_ENABLED=true`。已确认沿用原 Phase 5 企业服务时，按[已验证的兼容协议配置](R4_IDENTITY_DIAGNOSTICS_ZH.md#41-已确认沿用-phase-5-同一身份服务时)恢复原参数及登记回调，不能机械套用新模板。`SUBJECT_TENANT_SCOPED=false` 仅适用于主体 ID 在企业应用内全局唯一的情况；否则需配置租户字段。可设置 `REQUIRED_GROUPS` 或 `ALLOWED_EMPLOYEE_TYPES`，但这些是登录准入条件，**不会自动按部门批量加空间**。

### 5.2 默认模型

必须配置 Chat、Embedding、Rerank 三类服务：

- `MINDCREEK_MANAGED_LLM_*`
- `MINDCREEK_MANAGED_EMBEDDING_*`
- `MINDCREEK_MANAGED_RERANK_*`

每类均填写准确的 `NAME`、`BASE_URL`、`API_KEY`、`PROVIDER`。模板的 `generic` 用于兼容相应接口的服务；具体提供商按 WeKnora 支持类型选择。服务地址通常带 `/v1`，以模型服务实际契约为准，Rerank 需支持相应 rerank 请求。

`MINDCREEK_MANAGED_EMBEDDING_DIMENSION` 必须等于服务实际向量维数，不能按示例猜测。已有索引后改变向量模型或维度需要单独评估重建，不能直接当作无损升级。

如需 VLM，增加：

```dotenv
MINDCREEK_MANAGED_VLM_ENABLED=true
MINDCREEK_MANAGED_VLM_NAME=实际视觉模型名
MINDCREEK_MANAGED_VLM_BASE_URL=https://实际视觉服务/v1
MINDCREEK_MANAGED_VLM_API_KEY=实际密钥
MINDCREEK_MANAGED_VLM_PROVIDER=generic
```

保持稳定默认 ID：`builtin-mindcreek-chat`、`builtin-mindcreek-embedding`、`builtin-mindcreek-rerank`，可选 `builtin-mindcreek-vlm`。员工无需填写模型密钥；生成的 YAML 只引用环境变量，不含明文模型密钥。

### 5.3 内网模型白名单与内部 CA

将实际允许的模型域名或 IPv4 地址逐个填入 `SSRF_WHITELIST_EXTRA`，逗号分隔，**不带协议/端口/路径**。例如：

```dotenv
SSRF_WHITELIST_EXTRA=chat.company.internal,embedding.company.internal,rerank.company.internal
```

安装脚本只接受精确主机名/IP，拒绝 `*` 和网段。内部 gateway 已由 Compose 加入。不要放开整个内网来解决某个模型连接错误。

企业 HTTPS 证书由内部 CA 签发时，将根证书/所需中间 CA 保存为 `$MINDCREEK_STATE_DIR/ca/*.crt`（PEM）。`validate` 将公开 CA 与企业 CA 合并，挂到 gateway、app、DocReader。不能放私钥，也不能关闭 TLS 证书校验。

若企业批准某个模型在隔离内网用 HTTP，才显式设置 `MINDCREEK_MANAGED_ALLOW_HTTP=true`，并将准确主机加入白名单。企业 OAuth 默认保持 HTTPS，主站始终配置 HTTPS。

### 5.4 验证配置

```sh
python3 bin/mindcreek validate
python3 bin/mindcreek images
```

这一步检查占位值、模型维数/地址、AES 长度、秘密权限、精确主机白名单、Compose 结构和镜像 ID；**不代表真实 OAuth/模型已连通**。不建议直接运行 `docker compose config` 输出完整配置，它会展开服务秘密。需要结构检查使用 `python3 bin/mindcreek compose config --quiet`。

## 6. 初始化 admin 与默认空间

```sh
python3 bin/mindcreek install admin
python3 bin/mindcreek install status
python3 bin/mindcreek install default-space
python3 bin/mindcreek install status
```

预期阶段：`new → registering → admin_registered → account_ready → space_creating → space_ready → key_creating → ready`。具体中间阶段可能很快完成。

`install admin` 先关闭对外页面，启动私有依赖，短暂开放原生私有建号接口，只创建受控 admin，再关闭注册并通过原生启动逻辑授予平台管理员身份；最后清空临时 bootstrap 邮箱。正常网关始终不开放员工密码注册。不要把 app:8080 单独暴露为日常入口。

`install default-space` 使用 admin bearer 创建默认空间，并生成仅含 `manage_members` 的开户凭证。固定空间 ID 写入产品安装记录。两条安装命令可重复执行，不重复创建空间、不夺回已移交的 Owner。正常启动不会每次重新做安装。

`install status` 需要数据库和 gateway 依赖已启动。状态中不会打印密码和 Key；状态报告仍只向运维提供，不向匿名用户开放。

## 7. HTTPS 入口与启动

### 7.1 已有企业 HTTPS 代理（优先）

将代理上游指向应用机 `FRONTEND_BIND_IP:FRONTEND_PORT`。同机代理通常是 `http://127.0.0.1:18080`。保留 Host，传递原始协议，关闭响应缓冲以支持流式问答。Nginx 示例的业务段：

```nginx
location / {
    proxy_pass http://127.0.0.1:18080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto https;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header Connection "";
    proxy_buffering off;
    proxy_request_buffering off;
    proxy_read_timeout 600s;
    proxy_send_timeout 600s;
    client_max_body_size 500m;
}
```

证书、域名和监听 443 由企业代理配置，以上片段不是完整站点。启动：

```sh
python3 bin/mindcreek up
python3 bin/mindcreek ps
```

### 7.2 包内 Nginx TLS

将企业颁发的服务器证书链和私钥放置为：

```text
$MINDCREEK_STATE_DIR/tls/fullchain.pem
$MINDCREEK_STATE_DIR/tls/privkey.pem
```

证书 SAN 必须包含 `MINDCREEK_EXTERNAL_ORIGIN` 主机名，私钥 0600。包不生成企业生产证书。确认宿主 443 空闲后：

```sh
python3 bin/mindcreek up --tls
python3 bin/mindcreek compose exec -T tls nginx -t
```

TLS 默认监听 443，可用 `TLS_BIND_IP/TLS_PORT` 调整。若对外使用非 443 端口，Origin 与 OAuth 回调必须带相同端口。证书更新后检查配置并 `compose exec -T tls nginx -s reload`。若需要改变上传上限，要同时调整业务 `MAX_FILE_SIZE_MB` 与入口的 `client_max_body_size`；使用实例 override 或维护配置副本，不偷偷修改已校验包文件。

六个核心服务应为 healthy；TLS 容器为 running。容器设置 `restart: unless-stopped`，Docker 服务需由企业启用开机启动。之前手工 `stop` 的服务仍需再次 `up`。

## 8. 登录与企业联调验收

### 8.1 登录入口

- 员工：访问 `https://主站/`，恢复已有会话；没有有效会话时自动启动企业登录。首次登录自动建本地映射账户、以 Viewer 加入默认空间，不创建个人空间。
- admin：访问 `https://主站/admin/login`，使用初始化邮箱和密码。会话失效返回此入口。
- 员工被移出空间后，下次登录不会自动加回。有其他空间可继续使用；没有空间显示受限状态。
- admin 初始为平台管理员和默认空间 Owner。移交空间 Owner 不转移平台管理员身份。平台管理员不自动获得所有空间内容。

### 8.2 建议按此顺序调测

| 步骤 | 操作 | 通过条件 |
|---|---|---|
| 1 | admin 登录、刷新、退出、重新登录 | 身份正确；密码注册入口关闭 |
| 2 | 查看默认空间、重复运行安装命令后再次 `up` | 默认空间 ID 不变、成员不重复 |
| 3 | 新员工真实 OAuth 登录 | 唯一本地账户、默认 Viewer、仅对话导航 |
| 4 | Owner 加入/调整成员；批量邮箱预览后确认 | 有效企业邮箱映射正确；错误行不盲目提交 |
| 5 | Contributor 创建并编辑自有知识库，Viewer 尝试管理 | 行为符合固定原生角色规则；Viewer 无管理导航 |
| 6 | Owner 创建文档库，上传合成 Markdown/文本 PDF | 处理完成；原文、分块和引用能显示 |
| 7 | FAQ、新建 Agent、员工知识问答 | 使用服务端模型，检索/回答/引用正常；流式内容持续显示 |
| 8 | 第二个空间、非成员、混入无权 KB 请求 | 跨空间和混合越权请求拒绝 |
| 9 | 移除员工、再次登录；移交 Owner | 不自动加回；成员操作继续由 Owner 授权；平台身份不变 |
| 10 | 重启容器、停机备份和隔离恢复 | 安装状态、成员、文档与会话仍可用 |

admin/Owner 先使用合成内容联调。Wiki 需满足原生文档、模型与处理条件；图谱在本包关闭。扫描 PDF、复杂表格、超大文件、真实并发与模型质量单独记录，不以简单 Markdown 成功代替。

批量成员功能根据已知企业邮箱导入/预览，不包含部门目录自动同步；仅已开户且可映射的员工可直接添加，其他情况按邀请流程处理。原生成员角色变更/移除/邀请/Owner 移交要求 Owner，不能认为空间 Admin 有同样能力。所有权移交使用两步确认。

## 9. 状态、日志与常见问题

```sh
python3 bin/mindcreek ps
python3 bin/mindcreek install status
python3 bin/mindcreek logs gateway
python3 bin/mindcreek logs app
python3 bin/mindcreek logs docreader
python3 bin/mindcreek compose exec -T app curl -fsS http://127.0.0.1:8080/health
```

默认日志级别 warn，并关闭额外 LLM 调试文件，Docker 单服务日志轮转 10 MB × 3。原生代码在 info 级别会记录模型请求正文，因此只应在合成数据调测时临时启用 info/debug。错误日志仍可能包含用户邮箱、文件名或业务内容；向外提供排障资料前脱敏。不要上传实例 env、令牌或完整数据库转储。

| 现象 | 优先检查 |
|---|---|
| 镜像架构错误 / `exec format error` | 目标 `uname -m`、包版本、八镜像 AMD64 ID；不要换成 ARM 缓存 |
| `Image differs from the package lock` | 标签被另一套镜像覆盖；重新 `load` 当前包并复查 |
| `REPLACE` / 权限校验失败 | 模板未填完、env/秘密 0600、UID 与所有者、是否误在 root/普通用户之间切换 |
| admin 初始化卡住 | `install status`、App/PG 健康、管理员邮箱、DB 凭证；按下一节恢复，不删卷 |
| 首页重复跳转/登录回调失败 | Origin、DNS、证书、回调白名单、系统时间、浏览器 Cookie、Token/UserInfo 字段 |
| OAuth `x509: certificate signed by unknown authority` | CA 根/链是否放入 `ca/*.crt`；重新 `validate` 后重建 app/gateway；不要关闭证书验证 |
| 模型连接被 SSRF 拒绝 | 精确主机是否在白名单；DNS 是否指向预期地址；不要放行全网段 |
| 模型 401 / 404 / 400 | Key、服务路径、模型名、provider、实际 API 契约；错误 Key 不应静默替换 |
| Embedding 维数错误 | 服务实际向量维数与配置一致；已建库不能随意换维数 |
| 文档一直 processing/failed | DocReader/Redis/App 状态、文件大小、文档类型、VLM/外部依赖；查看具体任务错误 |
| 查询时数据库退出，出现 `munmap_chunk(): invalid pointer` / `signal 6` | 本机 AMD64 模拟已复现；先运行独立 `check-database`。目标物理 AMD64 仍需复核，不能直接归因为模拟器或修改默认索引掩盖问题 |
| 502 / 504 / 问答不流式 | 代理上游、gateway/app 健康、SSE 缓冲、代理超时、模型服务延迟 |
| 403 | 当前角色、成员关系、资源创建者、API Key 能力及 KB 范围；不要通过开放 app 绕过 |
| 410 `feature.retired` | 旧发布订阅/笔记 API 已退役；清理旧客户端调用 |
| HTTPS 启动失败 | 443 占用、证书与私钥匹配、证书 SAN、私钥读取权限；运行 `nginx -t` |

修改 env/CA 后先 `validate`，再执行 `up`（使用内置 TLS 时加 `--tls`）。**CA 和生成模型 YAML 是文件挂载，内容变化时需显式重建 app/gateway/docreader**，避免旧挂载/进程缓存：

```sh
python3 bin/mindcreek validate
python3 bin/mindcreek compose up -d --pull never --no-build --force-recreate --wait --wait-timeout 600 docreader app gateway
python3 bin/mindcreek up
```

只检查网络可以使用已有 app 内的 curl，或 `compose run --rm --no-deps ops` 执行 Python 探测。不要把模型密钥放在 curl 命令行或开启 `set -x`。

## 10. 安装中断与显式恢复

先读取状态，正常阶段先重试相同命令。远端创建结果不确定时，不自动创建第二个账户/空间/Key。

```sh
python3 bin/mindcreek install status
# 仅在核对实际原生账号后，显式采用正确的 UUID：
python3 bin/mindcreek install admin --admin-id 已核对的管理员UUID
# 仅在核对实际已创建空间/成员Key后：
python3 bin/mindcreek install default-space --default-space-id 已核对的数字空间ID --member-key-id 已核对的数字KeyID
```

`default-space` 恢复参数按实际阶段使用，不必总是同时传两个。不得猜测 ID 或改数据库表来跳过授权。

默认空间的成员 Key 丢失/撤销时，使用**当前空间 Owner**的人类 bearer，临时放入 `secrets/recovery-owner-bearer`（0600），执行：

```sh
python3 bin/mindcreek install repair-member-key --member-key-id 已核对的数字KeyID
```

具体 Key 选择需核对原生状态；修复完成后移除临时 Owner bearer 文件。Owner 已移交时，不能使用原 admin 的平台身份冒充当前 Owner，也不能重新夺回所有权。

管理员通过界面改密后，同步更新受控 `secrets/admin-password`，保证后续需要 admin 登录的维护命令使用正确凭证。更改密码不需要重新初始化账户。

## 11. 停机备份、恢复与升级

### 11.1 一致性备份

备份必须包含：原生 public schema + 产品 mindcreek schema 的完整数据库、原文件数据、Redis 队列状态、实例配置与密钥、包版本/镜像锁。只备份 PDF 文件或只备份产品表不能恢复。

以下是维护窗口内的示例。先通知用户停写；确认正在运行当前实例，再执行：

```sh
umask 077
python3 bin/mindcreek compose stop tls frontend gateway app docreader
python3 bin/mindcreek compose exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$MINDCREEK_STATE_DIR/backup/database.dump"
python3 bin/mindcreek compose exec -T redis sh -c 'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli SAVE'
python3 bin/mindcreek compose stop redis
python3 bin/mindcreek compose run --rm --no-deps --pull never ops -c 'import tarfile; t=tarfile.open("/backup/files-and-redis.tar.gz","w:gz"); t.add("/data/files",arcname="files"); t.add("/redis-data",arcname="redis"); t.close()'
tar --exclude=backup -czf "$MINDCREEK_STATE_DIR/backup/instance-secrets.tar.gz" -C "$MINDCREEK_STATE_DIR" .
cp RELEASE.json "$MINDCREEK_STATE_DIR/backup/RELEASE.json"
python3 bin/mindcreek up
# 若平时使用内置 TLS，上一条替换为 up --tls。
```

备份文件应加密存储并复制到异机受控位置，按企业策略保留多个时间点。本地 `backup/` 与服务同盘，不能作为唯一备份。不要将秘密备份放回公开离线镜像包。

### 11.2 恢复到隔离新实例

先在隔离主机或**新 project**演练；禁止对正在使用的卷直接执行覆盖恢复。使用相同包、模型配置、原 AES 与 JWT 密钥。

1. 导入镜像，设定新的 `MINDCREEK_STATE_DIR` 与 `MINDCREEK_PROJECT`；不要执行 `init` 生成新秘密。
2. 在新状态目录解压 `instance-secrets.tar.gz`。核对文件 UID/GID、0600/0700 权限，修正 env 中的数值 UID/GID。若只是备份恢复，保持 DB_USER/DB_NAME/DB_PASSWORD 与原值一致。
3. 将 `database.dump` 和 `files-and-redis.tar.gz` 放入新实例 `backup/`。若备份含本机 `compose.override.json`，先审查路径和网络映射。
4. 执行 `validate`，只启动 PostgreSQL，新卷中不要先启动 app/gateway 自动写入数据：

```sh
python3 bin/mindcreek validate
python3 bin/mindcreek compose up -d --pull never --no-build --wait --wait-timeout 600 postgres
python3 bin/mindcreek compose exec -T postgres sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --exit-on-error --no-owner' < "$MINDCREEK_STATE_DIR/backup/database.dump"
```

上面的数据库命令只用于**新隔离实例的目标库**，会删除备份中对应的现有对象。ParadeDB 初始化会预建 `paradedb` schema/扩展，因此恢复使用 `--clean --if-exists` 避免重复创建冲突；不能把这条命令指向在用企业库。文件恢复应使用下列明确路径映射的命令（仅信任企业自己生成、已校验的备份）：

```sh
python3 bin/mindcreek compose run --rm --no-deps --pull never ops -c '
import os,pathlib,tarfile,shutil
with tarfile.open("/backup/files-and-redis.tar.gz") as t:
    for m in t:
        parts=pathlib.PurePosixPath(m.name).parts
        if not parts or parts[0] not in ("files","redis") or ".." in parts or m.issym() or m.islnk():
            raise SystemExit("Unexpected backup entry")
        root=pathlib.Path("/data/files" if parts[0]=="files" else "/redis-data")
        p=root.joinpath(*parts[1:])
        if m.isdir(): p.mkdir(parents=True,exist_ok=True)
        elif m.isfile():
            p.parent.mkdir(parents=True,exist_ok=True)
            with t.extractfile(m) as source,p.open("wb") as dest: shutil.copyfileobj(source,dest)
        else: raise SystemExit("Unsupported backup entry")
        os.chmod(p,m.mode & 0o777)
        os.chown(p,m.uid,m.gid)
'
python3 bin/mindcreek compose up -d --pull never --no-build --wait --wait-timeout 600 postgres redis docreader app gateway
python3 bin/mindcreek install status
python3 bin/mindcreek up
```

状态应仍为 `ready`。验证原 admin、员工成员关系、文档读取/检索、后台任务和会话。恢复到新域名时先调整 Origin/OAuth 回调，旧浏览器会话和身份 provider 也要重新验收。不要对恢复后的数据再次执行首次建号逻辑。

### 11.3 升级与回滚

- 当前包以全新安装为范围，不做旧发布订阅/笔记数据迁移。
- 升级前备份并记录旧 `RELEASE.json`，停止业务，阅读目标版本迁移说明，在独立目录导入目标包。
- 维持同一 STATE_DIR 与 PROJECT 才会使用原卷。改 project 名会得到另一组空卷。
- 不把 Compose 配置或产品网关单独升级成不同 R 阶段组合；不使用浮动 `latest`。
- 数据库发生不可逆/不兼容迁移后，回滚需要恢复配套备份，不能只切换镜像标签。
- 日常停机用 `python3 bin/mindcreek stop`。`down --volumes` 会删除业务数据卷，仅用于明确授权的废弃测试环境；企业部署不要使用。

## 12. 调测记录与后续验收

保存：包 SHA256、镜像锁、OS/Docker 版本、配置结构（不含值）、域名/网络连通性、角色用例、模型请求错误码、文档处理耗时、并发测试、备份与恢复证据。

包内合成验收证明安装工具和镜像运行链路可执行；企业仍需完成真实 OAuth 映射、停用/撤权、真实 Chat/Embedding/Rerank/VLM、证书更新、磁盘容量、日志策略、备份恢复和物理 AMD64 性能验证。确认这些结果后再决定生产推广，不以历史 R2/R3/R4 测试替代目标环境验收。
