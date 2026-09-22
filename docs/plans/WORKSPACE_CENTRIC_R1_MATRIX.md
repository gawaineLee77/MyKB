# R1 功能、权限与模块对照表

日期：2026-09-15。依据批准的 WeKnora v0.8.0 和当前工作树。表中“目标”是 R2–R6 待实现行为，不代表当前产品已开放。

## 1. 功能对照

| 功能 | 当前产品与原生差异 | 目标及依赖 | 阶段 | 代码依据 |
|---|---|---|---|---|
| KB 列表、创建、详情 | 产品页面替换了原生页面，并引入个人模式和分享/订阅页签 | 恢复原生 KB 页面，保留品牌和模型适配 | R3–R4 | [覆盖脚本](../../tools/frontend-overlay/apply.mjs) |
| 文档/FAQ | 使用原生引擎，创建入口套用产品预设 | 原生上传、FAQ 导入、编辑、处理状态、重试及删除；需要存储、解析器、检索库和后台任务 | R3–R4 | [原生创建/设置](../../upstream/weknora/frontend/src/views/knowledge/KnowledgeBaseEditorModal.vue)、[产品预设](../../services/gateway/internal/preset/preset.go) |
| Wiki | 原生已有 Wiki 索引和生命周期，产品仍以旧模式为主 | 接回原生索引选项和页面；配置合成模型，验证任务、引用和删除；不沿用旧个人笔记 Wiki 的独立待办 | R3–R4 | [原生设置](../../upstream/weknora/frontend/src/views/knowledge/KnowledgeBaseEditorModal.vue) |
| 知识图谱 | 原生有图谱配置，当前产品关闭 graph 能力 | 支持原生配置；依赖 Neo4j、`NEO4J_ENABLE`、抽取模型和索引/查询生命周期验证。部署默认不开图谱，依赖齐备并通过验收后启用 | R3–R4、R6 | [Compose](../../upstream/weknora/docker-compose.yml)、[能力配置](../../config/phase5-capabilities.json) |
| VLM/OCR | 已有可选托管 VLM；产品创建预设负责注入 | 将模型注入接到原生创建/配置；扫描件验证留在 R4/R6；OCR 不等于 PixelRAG | R3–R4 | [模型服务](../../services/gateway/internal/managedmodel/service.go)、[预设](../../services/gateway/internal/preset/preset.go) |
| Agent | 原生 Agent 引擎与页面叠加产品知识范围检查 | 保留原生创建、编辑、运行和 Viewer 可运行设置；知识范围改用空间有效权限 | R3–R4 | [Agent 路由](../../upstream/weknora/internal/router/routes_agent.go)、[范围服务](../../services/gateway/internal/agentscope/resolver.go) |
| Viewer 对话 | 当前仍能看到 KB/Agent 入口 | 对话、历史、空间/Agent/KB 选择、引用预览及必要账户操作；管理导航按当前空间角色更新 | R4 | [菜单](../../upstream/weknora/frontend/src/stores/menu.ts)、[覆盖脚本](../../tools/frontend-overlay/apply.mjs) |
| 原生成员与共享 | 原生存在，但产品企业身份模式关闭部分邀请接口；旧 KB grant 与原生共享不是同一套授权 | 保留原生成员与资源共享；员工邀请经 OAuth；核对企业邮箱到上游身份邮箱的映射 | R2–R4 | [身份网关](../../services/gateway/internal/server/server.go)、[成员路由](../../upstream/weknora/internal/router/routes_auth_tenant.go) |
| 托管默认模型 | Chat/Embedding/Rerank 与可选 VLM 已实现 | 保留稳定 ID、服务端密钥和健康检查，接入原生 KB/Agent/Wiki 配置 | R3–R4 | [模型服务](../../services/gateway/internal/managedmodel/service.go) |
| MCP | 当前六个只读工具依赖旧授权链 | 保留可读 KB 列表、搜索、片段和问答四个工具，删除发布/订阅工具 | R3 | [MCP 服务](../../services/gateway/internal/mcp/service.go) |
| 网页小助手 | 原生有渠道和 Widget；产品 embed 关闭 | 渠道配置和界面可复用；增加真实员工会话适配，逐次检查成员、渠道、会话和引用权限 | R5 | [EmbedAuth](../../upstream/weknora/internal/middleware/embed_auth.go)、[短时令牌](../../upstream/weknora/internal/application/service/embed_session.go) |
| 成员批量加入/部门 | 原生成员 API 可逐人加入，没有企业部门映射 | R4 提供邮箱/CSV 批量调用；部门筛选和持续同步后置，不重建原生角色管理 | R4、后续 | [成员 API](../../upstream/weknora/internal/handler/tenant_member.go)、[身份数据](../../services/gateway/internal/identity/repository.go) |

图谱、Wiki、解析器和可选模型的“上游已有”不等于依赖已安装或问答质量已验证。原生 KB 支持范围需按本表在 R3–R4 逐项兑现。现有 ASR、外部连接器等排除项另列为兼容差异，不在 R1 静默开放；自研 PixelRAG、本体和桌面端不属于本次交付。

## 2. 四角色及平台权限

| 行为 | Owner | Admin | Contributor | Viewer | 目标阶段/依据 |
|---|---|---|---|---|---|
| 新建 KB/Agent | 允许 | 允许 | 允许 | 拒绝 | R3–R4；[原生路由](../../upstream/weknora/internal/router/routes_agent.go)及 RBAC |
| 修改已有 KB/Agent | 原生有效权限 | 原生有效权限 | 原生归属/共享权限 | 对适用的创建者/共享接口沿用原生权限 | R3；[RBAC](../../upstream/weknora/internal/middleware/rbac.go) |
| 查看成员列表 | 允许 | 允许 | 允许 | 原生 API 允许，产品管理导航收敛 | R2/R4；[空间路由](../../upstream/weknora/internal/router/routes_auth_tenant.go) |
| 添加/邀请/移除成员，修改角色 | 允许 | 拒绝 | 拒绝 | 拒绝 | R2/R4；原生 Owner 门槛 |
| 所有权移交 | 先增加接收方 Owner，再降级原 Owner | 拒绝 | 拒绝 | 拒绝 | R2/R4；[成员服务](../../upstream/weknora/internal/application/service/tenant_member.go)，不允许零 Owner |
| 使用知识/智能体 | 依成员与资源权限 | 同左 | 同左 | 同左；Agent 需对 Viewer 可运行 | R3–R4；[原生 Agent 类型](../../upstream/weknora/internal/types/custom_agent.go) |
| 管理网页渠道 | 原生 Admin+ | 原生 Admin+ | 拒绝 | 拒绝 | R5；[渠道路由](../../upstream/weknora/internal/router/routes_agent.go) |
| KB/Agent 菜单 | 展示 | 展示 | 展示 | 隐藏管理入口；保留对话内选择与引用 | R4；[当前菜单](../../upstream/weknora/frontend/src/stores/menu.ts)需产品适配 |
| 创建额外空间 | 仅角色本身不足 | 仅角色本身不足 | 拒绝 | 拒绝 | R2；企业策略要求平台管理员，[创建处理](../../upstream/weknora/internal/handler/tenant.go)需网关限制 |
| 全局托管模型/身份配置 | 仅空间角色不足 | 同左 | 拒绝 | 拒绝 | R2–R3；要求平台管理员，[身份管理](../../services/gateway/internal/server/identity.go) |

- 原生角色是固定枚举及等级；不新增自定义角色、批量重映射 Contributor 或增加统一 Admin 内容写入门槛。
- 平台管理员不自动获得所有空间内容权限；本次不启用跨空间超级用户开关。local admin 初始同时拥有平台权限和默认空间 Owner。
- 当前空间来自真实认证信息及原生成员关系；不能信任客户端单独传入的角色或空间 ID。
- 导航隐藏不是 API 权限变更。创建者降为 Viewer 后，具体资源操作仍遵循原生创建者/共享检查。
- 机器 `manage_members` 凭证用于开户，不能分配 Owner；不得替代人类 Owner 的所有权移交。

## 3. 模块处置

| 模块 | 当前职责 | 处置及顺序 | 阶段/依据 |
|---|---|---|---|
| 上游引擎、原生 RBAC/成员/共享 | 文档、索引、检索、Agent 与空间权限 | 保留批准版本和源码；接回原生工作流 | R2–R5；[上游路由](../../upstream/weknora/internal/router/routes_auth_tenant.go) |
| `identity`、broker、企业停用/注销 | OAuth2/OIDC 与企业账户绑定 | 保留；增加无空间开户和限定 local admin 的认证例外，恢复适用邀请接口 | R2；[身份门禁](../../services/gateway/internal/identity/gate.go)、[网关组合](../../services/gateway/internal/server/server.go) |
| `managedmodel` | 托管模型、默认值及密钥屏蔽 | 保留，适配原生 KB/Agent/Wiki 创建配置 | R3；[模型服务](../../services/gateway/internal/managedmodel/service.go) |
| `authorization`、`grant`、`library`、`agentscope` | 个人 KB、显式 grant、发布/订阅有效权限 | 新路径接管后解除旧链路；保留必要的检索前范围与会话检查 | R3；[组合入口](../../services/gateway/cmd/gateway/main.go)、[旧决策](../../services/gateway/internal/authorization/decision.go) |
| `publication`、`catalog`、`subscription` | 发布、目录、订阅 | 从新运行组合及 Web/MCP 入口退役；不再新增此类业务功能 | R3–R4；[组合入口](../../services/gateway/cmd/gateway/main.go) |
| `note`、`notespolicy` 与笔记 UI | 私有笔记、修订、配额 | 停用产品接口和入口；历史迁移/测试证据保留 | R3–R4；[组合入口](../../services/gateway/cmd/gateway/main.go)、[覆盖脚本](../../tools/frontend-overlay/apply.mjs) |
| `profile`、`preset`、`ingestion` | 旧模式、强制 Plain RAG 及上传策略 | 移除旧模式约束，保留必要上传/模型策略；适配社区导入工具 | R3；[预设](../../services/gateway/internal/preset/preset.go)、[导入工具](../../tools/community-import/README.md) |
| 产品 KB 页面覆盖 | 自定义创建、笔记、RAG、目录/订阅页面 | 原生页面接通后取消替换；保留品牌、OAuth、模型、角色导航覆盖 | R4；[覆盖脚本](../../tools/frontend-overlay/apply.mjs) |
| MCP、会话/引用范围 | 六工具及请求有效知识范围 | 四工具继续；授权和会话检查与 Web 一致 | R3；[MCP 服务](../../services/gateway/internal/mcp/service.go) |
| 路由策略、审计、观测、镜像和运维 | 产品边界及交付 | 保留；按阶段修订接口清单、记录事件和验收 | R2–R6；[路由配置](../../config/phase1-route-policy.json) |
| 测试与旧阶段记录 | 旧功能验收 | 有效测试复用；退役后再将旧功能断言改为接口关闭/边界检查；不重写历史“已通过”记录 | R3–R6；[历史索引](../archive/README.md) |

R1 只改变设计与验证工具，不停用上述业务模块或修改运行配置。新的授权链完整接管前，不能部署已移除旧鉴权的中间版本。
