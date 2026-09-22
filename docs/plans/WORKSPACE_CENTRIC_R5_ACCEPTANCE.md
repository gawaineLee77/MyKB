# R5 验收记录：员工网页小助手

日期：2026-09-22。状态：**R5 实现与合成验收完成**。企业目标域名、身份服务、模型、物理 AMD64 与发布验收仍属于 R6；本次没有部署到企业服务器，也没有更新此前的离线镜像包。

## 1. 实现与授权边界

| 入口/行为 | 实现及验证边界 |
|---|---|
| 渠道管理 | 产品 `/api/v1/mindcreek/assistant/agents/:agent/channels` 与 `/channels/:channel` 适配原生渠道；Owner/Admin 管理，过滤发布令牌等秘密字段 |
| 员工入口 | `/assistant/:tenant/:channel` 复用原生输入、流式消息、引用组件；`/assistant-login` 第一方企业登录弹窗 |
| 来源校验 | Nginx 动态 `frame-ancestors` 限制真实父页面；API 再核对原生渠道允许来源；窗口消息校验来源、窗口引用和随机 nonce |
| 问答 | 使用员工原始 bearer、固定渠道空间和服务端 Agent 配置；不接受客户端扩大知识范围、改选 Agent、公开资源链接参数 |
| 历史/文件/引用 | 每次核验员工、成员、渠道、Agent、会话及 KB；原生写/读取权限继续生效，不能通过主站或 MCP 绕过渠道绑定 |
| 本地 admin | 可以管理自己具备 Owner/Admin 角色的渠道；助手消费要求企业员工身份，不以 admin 代替员工 |
| 配额 | PostgreSQL 事务和渠道锁；每员工每分钟、渠道每天 UTC 配额，重启保留 |
| 迁移 | 新增 000016；会话与原生绑定同事务保存，非空数据拒绝破坏性回退；不修改历史迁移 |
| 停用 | 关闭配置或渠道后拒绝后续请求；不承诺实时切断已开始的流，不删除已经返回的内容 |

代码依据：`services/gateway/internal/assistant/`、`internal/nativeaccess/scope.go`、`cmd/gateway/native.go`、`tools/frontend-overlay/apply-assistant.mjs`、`tools/frontend-overlay/native/Assistant*.vue`、`EmployeeAssistant.vue`。完整操作见[部署使用指南](../guides/R5_EMPLOYEE_ASSISTANT_ZH.md)。

## 2. 证据清单

| 证据 | 含义 |
|---|---|
| [R5 基线](evidence/r5-baseline.json) | 固定上游版本，保存 R4 历史报告哈希 |
| [前端构建](evidence/r5-ui-build.json) | Node 24 单元测试、类型检查、构建与产物指纹 |
| [API 合成验收](evidence/r5-api-probe.json) | 独立实际 WeKnora/网关/数据库，合成身份与模型；包含本轮 R2/R3 回归 |
| [浏览器验收](evidence/r5-browser.json) | 真实 Nginx、Vue、Chrome 与原生 API；独立来源网页、合成 IdP 弹窗，无 API 响应拦截 |
| [工程检查](evidence/r5-checks.json) | 无缓存 Go、竞态、Node 24、Python 3.12、配置与上游边界 |
| [截图核对](evidence/r5-visual.json) | 9 张最终截图逐张检查，记录产物与截图指纹；重新截图或改 UI 后核对失效 |

结果：102 项 API/恢复检查、18 项浏览器检查、598 项前端测试及类型检查/构建、45 项 Python 3.12 导入工具测试通过。Go 全量测试使用 `-count=1`，并对授权、身份、会话、MCP 和助手执行竞态检查。设计/路由/配置检查通过；上游子模块无源码修改。构建保留原有大体积 JavaScript chunk 提示，不影响构建通过。

### 关键覆盖

- Owner/Admin 管理渠道；Contributor/Viewer 拒绝。匿名、本地 admin 消费、跨空间、跨员工会话、错误来源、客户端扩大 Agent/KB 和公开资源参数均拒绝。
- 合成 IdP → 首页中转 → 原生回调 → 员工开户 → 弹窗返回助手；首页中转现在保留 `state`，修复此前丢失该参数的问题。
- 流式完整回答、附件上传、历史图片重新授权和原生引用抽屉。接口另验证引用内容和文件读取，拒绝用例记录模型调用计数，确认拒绝发生在调用前。
- 员工停用/退出/移除、Agent 知识范围缩小、渠道关闭后拒绝后续请求及重连；主站普通会话路径不能绕过渠道绑定。
- 六个并发请求争用两次额度，验证数据库原子限额；重启后绑定与额度保留。
- 迁移 000013–000016 空库回退再升级；安装记录保护及恢复；有渠道会话时明确由 000016 拒绝破坏性回退。R2/R3 旧迁移的历史报告保留，本轮不将最新迁移拦截误标为单独重验了旧迁移所有拒绝分支。
- 本轮继续执行 admin 登录/刷新/改密/退出、移除不加回、所有权移交、开户密钥修复，以及 R3 原生资源、共享撤销、MCP/机器身份、默认模型和旧接口关闭回归。

浏览器对 iframe 的凭证存储读写施加限制，验证其使用内存中的员工 bearer；第一方弹窗仍保留 R2 主站会话存储。浏览器可能按自身策略共享同源存储，不将“iframe 没有写令牌”误称为“该来源的存储始终为空”。宿主网页没有收到令牌。真实企业 Cookie/COOP 策略仍需 R6 联调。

### 页面证据

| 页面 | 截图 |
|---|---|
| R4 智能体列表（变更前） | [加载完成的原页面](../assets/r5/before-agent-list.png) |
| 渠道管理与嵌入代码 | [发布设置](../assets/r5/after-channel-publishing.png)、[代码](../assets/r5/after-embed-code.png) |
| 企业登录与已登录助手 | [合成登录弹窗](../assets/r5/after-synthetic-login-popup.png)、[助手](../assets/r5/after-employee-login.png) |
| 问答、历史图片与引用 | [完整回答](../assets/r5/after-employee-answer.png)、[历史图片](../assets/r5/after-history-image.png)、[引用抽屉](../assets/r5/after-knowledge-references.png) |
| 渠道关闭 | [后续请求被拒绝](../assets/r5/after-channel-revoked.png) |

### 复验入口

```sh
make r5-ui-build             # R5_NODE_BIN 指向 Node 24
make r5-api-probe            # 独立临时 Docker 项目；需要 Playwright 和 Chrome
python3 tools/redesign/r5_checks.py
make r5-check               # 核对当前源码、产物、API/浏览器/工程和截图证据
```

浏览器依赖通过 `NODE_PATH` 或项目可解析的 Playwright 提供，`R5_CHROME_EXECUTABLE` 可指定 Chrome。先人工/代理逐张查看本轮截图并记录 `r5-visual.json`，再运行工程检查；不得只修改通过标记。合成配置、数据库和浏览器使用本机回环端口 18685–18688，执行完成仅移除临时项目；不接入企业服务。API 验收使用缓存的原生 ARM64 镜像，物理 AMD64 验收仍待 R6。

## 3. R6 交接

- 真实企业域名、OAuth2/CA/代理和 Cookie/COOP/CSP 策略；企业浏览器弹窗与不同域名网页。
- 企业模型的流式质量、图片理解、真实文档解析及引用质量。
- 目标物理 AMD64 的数据库预检、镜像版本化、备份恢复、负载、限流阈值和审计留存。
- 真实员工停用/离职同步及受控试点；不把本轮合成身份测试标记为生产身份验收。
- [七项后续功能](POST_R6_FEATURE_BACKLOG_ZH.md)继续排在 R6 收尾之后，不包含在本次实现。
