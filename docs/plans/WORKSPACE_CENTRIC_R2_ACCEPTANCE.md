# R2 验收记录：企业登录、默认空间与成员管理

日期：2026-09-15。状态：R2-1 至 R2-5 实现与本地合成验收已完成。尚未部署真实企业环境。

依据：[实施计划](WORKSPACE_CENTRIC_R2.md)、[权威设计](../OVERALL_DESIGN_ZH.md)、[安装与恢复手册](../../deploy/r2/README.md)。不回退 Phase 0，不修改 WeKnora v0.8.0 子模块。

## 1. 逐步交付

| 步骤 | 实现与结果 | 主要代码 |
|---|---|---|
| R2-1 | 安装记录、迁移 13、数据库安装锁；无空间 admin、私有注册/原生平台初始化、状态和显式 ID 恢复 | [安装服务](../../services/gateway/internal/enterprise/service.go)、[持久化](../../services/gateway/internal/enterprise/store.go)、[命令](../../services/gateway/cmd/gateway/install.go) |
| R2-2 | `/admin/login` 与网关 login/refresh/logout/change-password；校验固定安装 ID、有效状态和平台身份；未完成安装只进状态页 | [管理员认证](../../services/gateway/internal/enterprise/admin.go)、[页面](../../tools/frontend-overlay/product/mindcreek/AdminLogin.vue)、[初始化状态](../../tools/frontend-overlay/product/mindcreek/InstallationStatus.vue) |
| R2-3 | 人类 bearer 创建默认空间、保存固定 ID，仅有 `manage_members` 的服务凭证；重复运行不重新创建或夺回 Owner | [默认空间](../../services/gateway/internal/enterprise/workspace.go)、[当前 Owner 凭证恢复](../../services/gateway/internal/enterprise/recovery.go) |
| R2-4 | 迁移 14、用户锁、首次 Viewer、有效企业身份映射、结果不明时核对；完成记录不随移除清空；主站自动登录/开户 | [开户服务](../../services/gateway/internal/enterprise/onboarding.go)、[企业映射](../../services/gateway/internal/identity/account.go)、[入口](../../tools/frontend-overlay/product/mindcreek/enterprise-entry.ts)、[恢复页](../../tools/frontend-overlay/product/mindcreek/EmployeeOnboarding.vue) |
| R2-5 | 仅平台管理员人类 bearer 建空间；原生 Owner 成员/邀请接口及企业邮箱映射；四角色和两步移交保持原生规则 | [网关边界](../../services/gateway/internal/server/enterprise.go)、[前端覆盖](../../tools/frontend-overlay/apply.mjs) |

R2 通过 `MINDCREEK_ENTERPRISE_ENABLED=true` 启用；[新安装配置](../../deploy/r2/compose.enterprise.yml) 固定开启。历史 Phase 5 配置未自动切换。普通员工密码注册关闭；请求不能指定开户空间/角色；安装状态只对安装 admin 可读。客户端身份标记只决定跳转，后端不信任该标记。

## 2. 接口契约与权限

| 接口/操作 | 认证与行为 |
|---|---|
| `POST /api/v1/mindcreek/admin/auth/login` | 配置邮箱/密码，原生返回 `token`、`refresh_token`、用户与成员信息 |
| `POST /api/v1/mindcreek/admin/auth/refresh` | 请求字段是原生 **`refreshToken`**；响应 `access_token`、`refresh_token`；员工令牌拒绝并撤销新签发凭证 |
| `POST /api/v1/mindcreek/admin/auth/{logout,change-password}` | 安装 admin bearer；沿用原生撤销和密码策略 |
| `GET /api/v1/mindcreek/installation` | 仅固定安装 admin；返回非敏感阶段/ID，恢复通过命令执行 |
| `GET/POST /api/v1/mindcreek/onboarding` | 有效企业员工 bearer；服务器固定空间与 Viewer；无空间员工仅允许身份/开户/恢复相关请求 |
| `POST /api/v1/tenants` | 有效平台管理员人类 bearer；员工各空间角色及机器 Key 均不获建空间权限 |
| 原生成员/邀请写入 | 当前空间 Owner；Admin/Contributor/Viewer 不扩大权限；操作者原生 bearer 原样执行 |
| `install repair-member-key --member-key-id ID` | 命令读取临时当前 Owner bearer 文件；验证固定空间、Key ID/密钥/唯一作用域及实际可用性，不修改所有权 |

实现时发现并修正：此前设计中的刷新请求字段应为 camelCase `refreshToken`；原生系统设置的人类 bearer 需要有效空间，故先由环境变量关闭注册，建默认空间后再持久化系统设置；企业 provider 省略群组时必须保存空数组，不能写入违反 PostgreSQL 约束的 JSON null。

## 3. 可执行证据

| 检查 | 结果与边界 |
|---|---|
| [真实上游 API 探针](evidence/r2-api-probe.json) | 42 项检查、107 次请求通过；原生 v0.8.0 + 产品网关 + PostgreSQL + 合成 OAuth provider；随机项目、内部网络、无主机端口；只清理自身资源 |
| [网关及回归汇总](evidence/r2-checks.json) | 无缓存全套通过；新增安装/开户/权限契约，以及既有 OAuth 与模型相关包一起执行 |
| 定向 `go test -race -count=1` | enterprise、identity、server 三包通过；覆盖并发开户与鉴权代码 |
| Node 24 产品前端 | 测试、类型检查、构建通过；沿用上游依赖；大型构建块提示仍存在 |
| [Compose 检查](evidence/r2-compose-check.json) | 私有服务端口、loopback 前端、独立配置、关闭注册、tenantless 和企业开关通过；不是实际主机安装验收 |
| [浏览器记录](evidence/r2-ui-probe.json) | 9 个用例通过；5 张截图逐张检查通过，涵盖主站自动登录、有效会话、首次自动开户、管理员登录/过期和邀请 |
| Phase 5 / Stage 1 / 设计检查 | OAuth、默认模型、历史路由/模块和上游边界回归通过；中英文结构与文档链接检查通过 |

复跑命令：`make r2-api-probe`、`make r2-check`；Node 24 下运行 `make product-test-frontend PRODUCT_FRONTEND_TEST_ARGS=--offline` 和 `make r2-ui-probe`（需配置 Playwright 及浏览器路径）。汇总记录保存日志摘要指纹，API 记录绑定当前网关源码/二进制及探针指纹。

API 探针实际覆盖：空迁移回退再升级；拒绝抹除已有安装/开户记录后恢复 schema；并发首次加入仅一条成员；重复 OAuth 同一账号；移除后不加回及保留其他空间；Contributor 保留；Owner 邀请接受、四角色升降级与最后 Owner；跨空间拒绝；服务 Key 不可授予 Owner/访问其他空间；移交后重启不夺回；凭证撤销拒绝开户、新 Owner 显式恢复；管理员改密撤销 access/refresh。

故障注入单元测试另覆盖：外部写入前持久化标记、空间创建结果不明不盲目重试、指定空间 ID 必须实际拥有、成员结果不明先核对、数据库保存失败保留恢复路径、停用身份/映射冲突拒绝、精确 Key 作用域及混用凭证拒绝。这些为模拟故障证据，不标成真实进程任意时刻崩溃测试通过。

## 4. 页面证据说明

页面使用真实 Vue 构建、模拟 API 和合成用户。Before 为保持原样的企业登录组件、关闭 R2 开关；After 启用 R2 管理员/开户流程。浏览器全部请求限制到本机测试源，不连接公司 IdP 或真实模型。截图已检查：文字、表单、状态及操作显示完整，员工页无密码表单。

| 页面 | 截图 |
|---|---|
| Before：既有企业登录 | [截图](../assets/r2/before-employee-login.png) |
| After：独立管理员登录 | [截图](../assets/r2/after-admin-login.png) |
| After：安装状态 | [截图](../assets/r2/after-installation-status.png) |
| After：员工无可用空间 | [截图](../assets/r2/after-employee-removed.png) |
| After：开户失败与重试 | [截图](../assets/r2/after-employee-failed.png) |

## 5. 后续边界

- R3：替换旧授权链、移除发布订阅与笔记依赖、保留模型/OAuth、实现适用只读 MCP。
- R4：恢复 WeKnora 原生 KB/FAQ/Agent/成员界面，落实 Viewer 仅对话导航及批量成员交互。
- R5：员工网页小助手，保留真实员工会话权限。
- R6/实际部署：企业 IdP、TLS/反向代理、真实默认模型、目标主机安装命令、重启/备份恢复和企业试点单独验收。

本轮只使用合成身份/数据和缓存镜像，没有连接企业账号、真实模型或现网数据库，没有执行企业部署或提交上游补丁。
