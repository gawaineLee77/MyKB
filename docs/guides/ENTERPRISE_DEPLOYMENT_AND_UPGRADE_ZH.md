# 最新 AMD64 完整部署包：全新安装、升级与恢复

本指南对应 `current-*` 完整离线包，包含当前 R5、Neo4j 知识图谱展示、单条回答 Word 导出及之前的企业登录、默认模型、链接隐藏修复。旧 R4 包和补丁继续保留，不再逐个叠加补丁。

## 1. 包含什么、适用什么环境

- 目标：Linux x86_64 / AMD64、Docker Engine（本次验证 29.3，需支持 `docker image inspect/save --platform`）与支持 `up --wait` 的 Compose v2/v5（本次验证 5.1）、Python 3.10+。服务器无需 Go、Node、源码或联网拉取镜像。
- 九个镜像：产品前端、产品网关、WeKnora App v0.8.0、DocReader v0.8.0、ParadeDB PostgreSQL 17、Redis 7.0、Nginx、Python 运维工具、Neo4j 2025.10.1（含 APOC）。镜像精确 ID、源代码指纹见 `RELEASE.json`。
- 全新安装默认启动六个核心服务；可选内置 TLS、Neo4j。图谱关闭时镜像仍包含在包里，便于后续离线开启。没有模型权重；Chat、Embedding、Rerank 以及可选 VLM 使用企业已有服务。
- 镜像打包了当前工作树，不仅是 Git HEAD。图谱展示面向 Owner/Admin/Contributor；Word 导出面向所有能查看回答的用户。图谱人工修订、Wiki 下载、定时解析等待办不包含在本版。
- 升级支持已有 **WeKnora v0.8.0 + R2–R5 原生空间企业包**，包括此前 R4 身份、图谱和链接补丁。更早的 `mindcreek-phase5` / WeKnora v0.7.2 / 发布订阅与个人笔记数据库不支持直接升级，工具会拒绝；需另行制定数据迁移，或使用独立空实例全新安装。

建议调测起点：8 核、16 GB 内存（开启图谱及并发解析建议 32 GB）、至少 100 GB 可用磁盘，另按实际数据和备份预留容量。这是容量规划起点，不是性能承诺。冷备份需要容纳全部卷的额外空间；保留至少一个可恢复的历史包。

## 2. 拷贝、校验与导入

将 `.tar.gz` 及同名 `.sha256` 一起传到服务器；不要修改包内文件，不要把实例秘密放进包里。

```sh
sha256sum -c mindcreek-current-版本-amd64.tar.gz.sha256
tar -xzf mindcreek-current-版本-amd64.tar.gz
cd mindcreek-current-版本-amd64
python3 bin/mindcreek verify
python3 bin/mindcreek load
python3 bin/mindcreek images
```

实际文件名以交付物为准。`load` 不启动业务，不删除旧镜像；容器运行使用 `pull_policy: never`。包内 `SHA256SUMS` 和外层 SHA256 用于发现传输损坏，并不是数字签名。

在导入业务数据前，运行隔离的数据库检查：

```sh
python3 bin/check-database --output /安全目录/mindcreek-database-check.json
```

本机 ARM Mac 上的 AMD64 模拟曾出现 BM25/PostgreSQL 退出问题，不能将模拟器中的向量检索成功当作混合检索通过。以本包验收记录为准；企业物理 AMD64 必须运行此检查。失败时先停下排查，不导入业务数据，也不自动更改原生索引策略。

## 3. 从零安装

### 3.1 固定实例目录和项目名

```sh
export MINDCREEK_STATE_DIR=/opt/mindcreek/instances/main
export MINDCREEK_PROJECT=mindcreek-enterprise
python3 bin/mindcreek init
```

`init` 交互输入本地 admin 初始密码，并生成随机数据库、Redis、JWT、AES 和桥接密钥。不会覆盖已有实例。admin 密码为 12–32 字符，包含字母和数字；企业 OAuth Client Secret 使用身份服务发放的原值，不要求自行扩长。

使用同一个 Linux 运维账户执行后续命令；保存这两个环境变量。实例目录 0700，`enterprise.env`、`secrets/*` 0600。`DB_NAME=mindcreek_r4` 是兼容保留的数据库名称，不表示运行旧版。

### 3.2 编辑企业配置

编辑 `$MINDCREEK_STATE_DIR/enterprise.env`，不能直接把示例值用于企业部署：

| 配置 | 填写要求 |
|---|---|
| `MINDCREEK_EXTERNAL_ORIGIN` | 如 `https://mindcreek.com`，**末尾不带 `/`**，不带路径；避免网关与 App Issuer 不一致 |
| `MINDCREEK_INSTALL_ADMIN_EMAIL` | 独立本地 admin 邮箱；建议与企业员工邮箱分开 |
| `MINDCREEK_IDENTITY_*` | 企业授权、Token、UserInfo URL、Client ID/Secret、请求格式、稳定主体和邮箱字段；沿用已确认的实际协议 |
| `MINDCREEK_IDENTITY_REDIRECT_URI` | 建议显式设置 `https://mindcreek.com/api/v1/mindcreek/oidc/callback`，并在企业身份应用登记完全相同地址 |
| `MINDCREEK_MANAGED_LLM_*` | Chat 模型名称、Base URL、Key、Provider |
| `MINDCREEK_MANAGED_EMBEDDING_*` | 向量模型配置及真实维数，已有索引不能直接换维数 |
| `MINDCREEK_MANAGED_RERANK_*` | Rerank 服务配置；可选 `MINDCREEK_MANAGED_VLM_*` 用于视觉模型 |
| `SSRF_WHITELIST_EXTRA` | 允许访问的模型主机名/IP，逗号分隔，不能带协议/端口/路径/通配符 |
| `MINDCREEK_EMPLOYEE_ASSISTANT_ENABLED` | 本包已含 R5，可设置 `true` 启用网页发布；每个渠道仍需配置来源和访问权限 |

对于之前已调通的企业身份服务，**升级保留原配置**。不要把 `globalUserID` 改成 `sub`、不要自行切换 UserInfo Token 传递方式、state/PKCE 或 OAuth2/OIDC 模式。UserInfo 邮箱字段为 `email` 时保持对应映射，实际唯一身份依赖稳定 subject/tenant，不以邮箱代替。参见[身份排障](R4_IDENTITY_DIAGNOSTICS_ZH.md)。

### 3.3 企业 CA 与 HTTPS 服务证书

```text
$MINDCREEK_STATE_DIR/ca/certificate-ca.crt   企业根/中间 CA，PEM 格式
$MINDCREEK_STATE_DIR/tls/fullchain.pem      主站服务器证书链（仅内置 TLS 需要）
$MINDCREEK_STATE_DIR/tls/privkey.pem        主站私钥，0600（仅内置 TLS 需要）
```

`ca/*.crt` 会在 `validate` 时与公开信任根合并，供 gateway、app、DocReader 使用。原 `ca.pem` 改名为 `certificate-ca.crt` 可以，内容必须是 PEM 证书；目录中不能放私钥。主站 TLS 证书与企业身份/模型的 CA 信任用途不同，不能互相替代。

### 3.4 初始化与启动

```sh
python3 bin/mindcreek validate
python3 bin/mindcreek install admin
python3 bin/mindcreek install default-space
python3 bin/mindcreek install status
python3 bin/mindcreek up
python3 bin/mindcreek ps
```

安装状态应为 `ready`。重复安装命令不会创建第二个默认空间，也不会夺回已经移交的所有权。员工首次登录自动加入默认空间 Viewer；被移除的员工不会在下次登录时自动加入。

如使用包内 TLS，准备证书后，将 `up` 换为 `up --tls`。具体安装恢复参数和受控 member-key 修复，见[安装与恢复说明](R4_AMD64_ENTERPRISE_DEPLOYMENT_ZH.md#10-安装中断与显式恢复)。升级流程不运行首次安装命令。

## 4. 域名与反向代理

默认只发布 `127.0.0.1:18080`，可以由**同机** Nginx/企业代理转发到域名。异机代理不能访问这台服务器的回环地址，应将 `FRONTEND_BIND_IP` 设置为指定内网接口，并限制只有代理可访问。不能把 app、PostgreSQL、Redis、Neo4j 直接暴露为员工入口。

同机 Nginx 已配置 HTTPS 证书时，可使用业务段：

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

`/assistant/` 的 CSP 由产品网关生成。外层代理不能给此路径追加禁止嵌入的 `X-Frame-Options` 或冲突的 `frame-ancestors`。域名、DNS、证书 SAN、浏览器与 IdP 的回调白名单必须一致。

## 5. 可选能力

### 知识图谱（包内已含 Neo4j/APOC）

新实例或本包管理的实例执行：

```sh
python3 bin/release graph enable
python3 bin/mindcreek validate
python3 bin/mindcreek up
# 使用内置 TLS 的实例始终保留 up --tls
```

工具生成私有图数据库凭证、配置 Neo4j、开启抽取和只读图谱展示，保留已有图谱密码与卷。不会下载 APOC。已有旧图谱补丁由升级工具自动接管，不要再次执行旧 `graph.py enable`，否则会重新固定旧网关镜像。

图谱服务默认仅容器网可达；不发布 7474/7687。支持在知识库配置中启用抽取、执行处理，并在“知识图谱”页查看实体关系；与 Wiki 页面链接图分开。关闭用 `python3 bin/release graph disable` 后 `up`，不会删除图谱数据。自建外部 Neo4j 配置需单独审查，自动工具不会接管。

### 智能体发布与 Word 导出

- 设置 `MINDCREEK_EMPLOYEE_ASSISTANT_ENABLED=true` 后执行 `up`，Owner/Admin 可发布渠道。参见[网页小助手](R5_EMPLOYEE_ASSISTANT_ZH.md)。宿主网页中的标识不是权限凭证，员工仍需企业登录。
- Word 导出无需额外服务器服务或参数。主站单条回答的下载按钮导出可编辑表格和支持的公式；图片暂为说明，不支持的公式会报错。参见[Word 导出](CHAT_WORD_EXPORT_ZH.md)。
- 外部帮助链接默认隐藏；原有 `$STATE/ui-links/links.json` 在升级时保留。参见[链接配置](R4_ENTERPRISE_LINKS_ZH.md)。

## 6. 已有企业实例升级（保留数据）

### 6.1 升级前核对

在**新包解压目录**操作，保留旧包目录用于恢复。必须准确沿用原来的 `STATE_DIR` 和 Compose 项目名；从旧命令、旧运维记录或 `docker compose ls` 核对，不能猜测。项目名变更意味着不同数据卷。

```sh
export MINDCREEK_STATE_DIR=/原来的/实例目录
export MINDCREEK_PROJECT=原来的compose项目名
python3 bin/mindcreek load
python3 bin/release plan --from-package /原来的/完整包目录
```

`plan` 不修改配置或数据，报告版本、卷名和将移除的历史镜像覆盖。它保留企业 OAuth、默认模型、CA、admin、JWT/AES、原文件、UI 链接、成员关系和 Neo4j 密码。对于未知镜像、策略文件或前端整目录挂载会停止，要求先审查，不能忽略错误强行启动。

升级前将 Origin 的末尾 `/` 去掉（若还有）；其他已调通身份参数保持不变。若原服务不属于本指南支持的企业包，不能通过伪造 `RELEASE.json` 绕过检查。

### 6.2 维护窗口内执行

先停止用户写入并暂停外部流量。备份路径必须是**尚不存在的新目录**，且位于实例外或实例的 `backup/` 子目录；建议使用独立磁盘。

```sh
python3 bin/release upgrade \
  --from-package /原来的/完整包目录 \
  --backup /安全备份目录/mindcreek-before-current-日期时间
python3 bin/mindcreek ps
python3 bin/mindcreek install status
```

工具执行：配置预检 → 停止该项目全部服务 → 冷备份所有具名卷和实例配置 → 移除已识别的旧 UI/网关镜像覆盖 → 更新图谱路由/能力文件 → 以新镜像启动 → 确认安装仍为 `ready`。此前运行内置 TLS 的实例会恢复 TLS 服务。备份包括原生和产品数据库、文件、Redis 队列、DocReader 临时文件及可选 Neo4j 卷，记录校验和和原始版本。

网关按顺序应用缺失的产品迁移，包括 R5 渠道会话结构；不重新注册 admin、不重建空间、不更换模型 ID、不改动旧知识索引。平台管理员身份与空间 Owner 仍分离。

**失败处理：**工具保留现有卷，不自动切回旧网关。检查备份完整性及 `$STATE/upgrade-pending.json`（若存在）和日志。若只是外部网络/配置问题，修正后可用新包 `up`；若需要回退，按下一节恢复成套备份。不能只运行旧前端/网关绕过新增渠道权限绑定。

完成登录、知识读取、上传、检索、权限和新功能验收后再开放流量；复制备份到异机加密存储。新包不能仅替换为旧 `latest` 标签。

## 7. 日常备份与隔离恢复/回退

### 7.1 停机冷备份

```sh
python3 bin/release backup --output /安全备份目录/mindcreek-日期时间
python3 bin/mindcreek up
# 若使用内置 TLS：up --tls
```

备份成功后服务保持停止，需运维显式恢复启动；备份失败同样不会自动恢复流量。`INCOMPLETE.json` 存在表示备份未完成，不得用于恢复。备份含密钥和企业数据，必须受控保存；不要发送给外部排障人员。备份使用固定数据库版本的冷卷快照，不用于跨 PostgreSQL/Neo4j 主版本迁移。

### 7.2 恢复到新的隔离实例

恢复必须使用备份记录的**同一个旧版本包及镜像**。可以使用新包的恢复工具指定旧包；先从旧包导入镜像，再执行：

```sh
export MINDCREEK_STATE_DIR=/opt/mindcreek/instances/recovery-日期
export MINDCREEK_PROJECT=mindcreek-recovery-日期
python3 /新包目录/bin/release --package /备份对应的旧包目录 restore \
  --backup /安全备份目录/mindcreek-before-current-日期时间
```

工具拒绝原项目、原实例目录、已有容器或卷，校验备份后恢复**新卷**。备份还包含当时实际运行的 UI/网关镜像，恢复工具会导入这些镜像以满足旧补丁覆盖；旧完整包仍需保留并导入其基础镜像。SHA256 校验不是来源认证，只使用企业自己生成的可信备份。恢复不会启动服务。

使用恢复时设定的两个环境变量，进入对应旧包目录；修改新实例的监听端口，核对 UID/GID、CA、TLS 和 Origin。将 `MINDCREEK_UID/GID` 调整为持有恢复秘密文件的运维用户；AES/JWT/数据库/模型/身份密钥不得重新生成。初次使用隔离入口，避免两套服务同时接收企业请求。

```sh
cd /备份对应的旧包目录
python3 bin/mindcreek validate
python3 bin/mindcreek compose up -d --pull never --no-build --wait --wait-timeout 600 postgres redis docreader app gateway
# 若有图谱，先启动 neo4j 并确认健康，再启动 app/gateway。
python3 bin/mindcreek install status
python3 bin/mindcreek up
```

旧 R4 `up` 在全部服务停机时需先启动依赖，上述命令兼容这个限制。新包已修正。恢复的旧图谱/UI 覆盖依赖对应补丁镜像；本工具冷备份已保存当时实际运行的 UI/网关镜像。旧版手工备份没有这项内容时，仍须保留对应补丁包。本次新包包含相同 Neo4j 镜像。

核验安装、原成员/空间/文档/会话、图谱和模型后，在维护窗口将企业代理切换到恢复实例。原失败实例保留用于排查，未经授权不得删除旧卷。回退会丢失备份时点之后写入的新内容，因此升级期间应保持停写。

## 8. 调测清单与排障

| 项目 | 必查结果 |
|---|---|
| admin | `/admin/login` 登录、刷新、改密、退出，普通注册接口仍关闭 |
| 员工 | `/` 自动企业登录、首次 Viewer 开户；移除后不自动加入 |
| 成员/空间 | 平台 admin 建空间；Owner 成员管理/移交；Admin 不获得 Owner 专属操作 |
| 知识与模型 | 原文/FAQ/分块、真实向量和混合检索、流式问答与引用；模型默认 ID 保持 |
| 图谱 | 已有实体/关系不丢失；Owner/Admin/Contributor 可查看，Viewer 不可管理 |
| 小助手 | 实际宿主来源、OAuth 弹窗、员工身份与撤权、动态 CSP |
| Word | 主站导出、表格和公式在已激活企业 Word 中编辑并保存 |
| 升级数据 | 旧 KB、成员、模型引用、图谱、文件与历史会话保持；备份恢复可执行 |

```sh
python3 bin/mindcreek logs gateway
python3 bin/mindcreek logs app
python3 bin/mindcreek logs docreader
python3 bin/mindcreek ps
```

CA/模型 YAML 变更后先 `validate`，再显式重建文件挂载的服务，最后 `up`：

```sh
python3 bin/mindcreek validate
python3 bin/mindcreek compose up -d --pull never --no-build --force-recreate --wait --wait-timeout 600 docreader app gateway
python3 bin/mindcreek up
```

不要公开输出 `compose config` 的展开值、Token、模型密钥或企业文档；结构检查用 `compose config --quiet`。不要执行 `down --volumes`、删库或重新 `init` 来修复升级故障。

包内 `evidence/` 记录本次真实镜像、合成安装/升级/恢复验证及限制。企业 OAuth、真实模型质量、Word 编辑保存、物理 AMD64 性能和生产推广仍属于目标环境验收，不由本地合成测试代替。
