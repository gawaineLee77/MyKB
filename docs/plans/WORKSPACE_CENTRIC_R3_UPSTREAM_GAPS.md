# R3 原生接口缺口与最小上游提案

状态：调查与提案，不构成上游补丁批准；上游源码保持干净。遵循 [补丁准入规则](../UPSTREAM_PATCHES.md)。

## 1. 同秒重新登录返回已经撤销的 JWT

依据：`upstream/weknora/internal/application/service/user.go` 的 `generateTokensForTenant` 仅使用秒级 iat/exp 和固定用户字段生成 access/refresh claims，没有独立签发 ID。R3 首轮安装确认后立即登录的合成请求中，原生 login 成功，但随后的 auth/me 报 bearer revoked。

产品适配：只对密码验证成功、新 bearer 在 auth/me 返回 401 的情况等待 1.1 秒并重新做一次完整密码登录；再次核验固定安装 admin、active 与平台管理员。安装命令复用该逻辑。刷新令牌不重试，不绕过撤销检查。员工 OAuth2 登录仍由原生流程处理；尚需补充其同秒注销重登的边界证据。

最小提案：每次签发生成独立随机 `jti`，access 与 refresh 均唯一；增加同秒登录/注销/刷新/空间切换的撤销隔离测试。需认证架构审查；上游具备唯一签发 ID 并通过回归后移除产品等待重试。适用固定 v0.8.0。

## 2. 聊天历史向量检索与统计缺少会话范围接口

依据：`upstream/weknora/internal/application/service/message.go` 的 `vectorSearchViaKB` 先检索整个聊天历史 KB，再按 `SessionIDs` 过滤消息；`GetChatHistoryKBStats` 返回空间聊天历史 KB 的整体计数。原生所有权过滤发生在取回之后，无法用公开请求字段保证 R3 的“检索之前按产品主体与绑定范围过滤”。

已调查替代：原生 keyword 消息搜索可以在数据库查询中限制 OwnerID 与 SessionIDs；产品补入已绑定、当前仍可访问的 session IDs。原生向量/混合历史搜索以及启用历史 KB 后的整体统计缺少同等的前置范围接口，不能通过替换凭证或省略检查解决。

产品边界：消息读取、继续问答和原生 keyword 历史搜索保留；vector/hybrid 历史搜索返回 409 `history.vector_scope_unavailable`，启用聊天历史 KB 后的整体统计返回 409 `history.aggregate_scope_unavailable`。这些是原生可选聊天历史索引接口，不影响知识库搜索、RAG 问答、文档、FAQ、Wiki 或普通会话历史读取。R4 搜索界面应明确使用 keyword 并处理范围为空状态。

最小提案：公开接收受限 session IDs，由当前 native principal 的会话所有权二次缩小；在向量检索/重排之前应用对应 knowledge IDs/owner/session 元数据约束。统计同样按授权 session IDs 聚合；空集不得退化成整空间。增加两 Key、员工、已删除成员、旧会话混入的检索调用证据。涉及检索/授权，需架构审查；上游公开接口通过验证后移除相应 409 边界。禁止直接修改上游实现。


## 3. 同 ID Agent 的来源选择

原生 `GET /agents/:id` 只读取当前空间；共享 Agent 配置可从 `GET /shared-agents` 的 `agent` 字段取得，R3 已使用这一公开替代，不需要提升凭证。普通 UUID Agent 的共享与撤销已纳入合成验收。

原生聊天支持 `agent_source_tenant_id`，但 Agent 详情和按 Agent 枚举 KB 的公开接口没有相同的来源参数。在多个空间出现相同内置 Agent ID 时，不能用当前空间配置预检后执行另一份配置。R3 对冲突来源返回 409 `agent.source_ambiguous`；不静默改选，也不切换为来源空间的管理凭证。

最小提案：详情与 Agent KB 枚举公开接受相同来源字段，由原生成员/共享权限确认，再与聊天使用同一解析逻辑。增加本地内置 ID 和两个共享来源重名的测试。涉及授权，需要架构审查后才可应用；可优先使用唯一 ID 的自定义 Agent。

## 4. Agent 内部的历史检索

`internal/agent/tools/search_conversations.go` 调用 hybrid `SearchMessages`，不接收产品绑定的 session IDs；空 `allowed_tools` 的推理 Agent 会采用包含此工具的原生默认列表。仅过滤外层 `/messages/search` 无法覆盖内部调用。

R3 在执行前拒绝 `search_conversations` 和隐式默认工具列表；quick-answer 及明确的知识/Wiki 工具列表可用。MCP 另外拒绝可能写知识、执行数据库/外部工具、技能或记忆的 Agent。最小上游提案是将受限会话谓词传入 Agent 工具构造与检索服务，复用第 2 项的检索前约束。不要通过重写 Agent 原生配置或改用 Owner 身份绕过。

## 产品侧回归修复

实际 PostgreSQL 停用接口验证发现产品 `identity.Repository.SetStatus` 的 CASE 参数需要显式 `varchar` / `timestamptz` 类型；已修正 SQL 参数绑定，未改历史迁移或上游。停用后检索拒绝、重新激活及成员移除均重新验证。此项属于产品缺陷修复，不是上游补丁申请。
