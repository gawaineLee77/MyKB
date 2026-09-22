# R3 验收记录：原生空间授权接管

状态：**R3 后端/API 实现与合成验收完成（2026-09-15）**。原生页面、Viewer 导航和批量成员在 R4；本轮不作为完整产品部署或真实企业环境验收。

## 1. 基线与证据

- [R2 原验收记录](WORKSPACE_CENTRIC_R2_ACCEPTANCE.md)及其四份证据保持原样；[R3 基线](evidence/r3-baseline.json)保存开始时 164 项既有工作树状态、上游及 R2 文件摘要，不清理已有工作。
- 固定 WeKnora v0.8.0、`1edcd54b43606d9079bb36650efe3f68707a79ea`；上游源码无修改。产品代码不导入上游 `internal/**`。
- [API/MCP 报告](evidence/r3-api-probe.json)：**86 项检查、229 次请求**；包含本轮重新执行的 R2 回归、预期/实际状态、检索与模型计数、镜像 ID、网关二进制和源码指纹。
- [工程报告](evidence/r3-checks.json)：10 组检查全部通过，保存命令、退出码、耗时和日志摘要。`make r3-check` 将证据与当前 Go、配置、验证工具、导入工具及 UI overlay 指纹核对。
- 使用随机 Compose 项目、内部网络、独立 PostgreSQL/Redis/文件数据及缓存镜像。员工、OAuth 服务和模型均为合成数据；只清理本次创建的项目，不连接公司身份/真实模型，不删除历史卷。

## 2. 六步结果

| 步骤 | 结果 | 交付与依据 |
| --- | --- | --- |
| R3-1 | 完成 | [路由/权限表](WORKSPACE_CENTRIC_R3_ROUTES.md)、429 条精确路由、独立 R3 配置/安装入口、R2 历史指纹 |
| R3-2 | 完成 | `nativeaccess.Gate` 与独立 `nativeDependencies`；真实凭证进入原生 KB、文档、FAQ、Agent、共享写接口；无旧 profile/grant 前提 |
| R3-3 | 完成，保留明确的公开接口限制 | 原生可访问 KB、服务端 Agent 配置、检索前检查、主体会话绑定、引用/文件检查、重连与撤销；历史向量/Agent 歧义见第 7 节 |
| R3-4 | 完成 | 缺省托管模型、显式模型校验、合法索引/Wiki 配置保留、文档上传策略；图谱依赖不足返回明确错误 |
| R3-5 | 完成 | 退役接口 410；四工具 MCP；原生社区建库/上传/重解析与不确定结果恢复 |
| R3-6 | 完成 | 本轮 API/MCP、迁移/恢复、无缓存 Go、竞态、Node 24、Python 3.12、配置/文档/边界检查；[R4 清单](WORKSPACE_CENTRIC_R4.md) |

## 3. 权限与范围证据

| 范围 | 本轮验证 |
| --- | --- |
| 四角色及创建者 | Owner 创建；Contributor 创建/编辑自有 KB、修改他人 KB 拒绝；创建者降为 Viewer 后仍可编辑；普通 Viewer 创建/修改他人 KB 拒绝；空间 Admin 修改资源允许 |
| 原生资源 | 原生 KB 无 profile 创建成功；Agent 创建/模型配置、Viewer 删除他人 Agent 拒绝、Owner 删除；原生 FAQ 创建/读取及 Viewer 写入拒绝 |
| 文件 | Markdown 文件上传并完成原生处理；手工 Markdown 发布/处理成功；Viewer 预览允许而原文件下载拒绝，Contributor 下载允许 |
| 共享 | 两个空间通过原生组织共享 KB；共享 Viewer 写入拒绝；撤销后历史、继续问答、流式重连拒绝；Agent 共享配置从原生共享列表解析，撤销后范围拒绝 |
| 检索前拒绝 | 混入跨空间 KB、Agent 范围扩大、共享撤销、员工停用、MCP 写内容 Agent 等拒绝；报告中的 `execution_counts` 记录原生检索入口与模型调用计数，确认对应拒绝没有进入执行 |
| 会话 | 员工之间、两个 Key 之间、Key 与员工之间拒绝串用；本主体绑定列表；旧会话不接纳；KB 范围并集绑定；网关重启后绑定仍有效 |
| 流及引用 | 正向原生问答返回答案和引用，来源片段可读取；单元测试验证逐事件检查，越权 SSE 帧在输出前截断、超长帧有界拒绝；不宣称已经开始的流即时撤销 |
| 平台与成员 | 新一轮 R2 验证平台建空间、Owner 成员/邀请接口、最后 Owner、两步所有权移交、移交后不夺回 Owner、被移除员工不自动加回 |

权限来源仍是原生对应写接口。产品的读预检不授予写权限；平台管理员不自动取得所有空间内容权限。API Key 不套用返回用户的 Owner 身份，按原生 capability 与 KB scope 检查。

## 4. MCP 与导入工具

保留 `list_knowledge_bases`、`search_knowledge`、`get_source_excerpt`、`ask_knowledge_agent`。匿名发现拒绝；删除的工具返回标准 Unknown tool 错误；受限 Key 无能力或超出 KB 范围时拒绝。

身份管理和产品模型管理只接受人类会话；受限 Key 调用这两个产品管理入口会在解析上游 Owner 用户前拒绝，并经过本轮实际接口测试。

机器主体使用不可逆 Key 指纹，不保存明文 Key；限流、会话和审计独立。Key 撤销后失效，另一 Key 不受影响；移除员工不自动撤销独立机器 Key。执行始终传原始凭证，不替换管理员凭证。受限 `chat` Key 无需读取模型管理接口即可在原生问答内部使用固定托管模型；MCP 不接收模型覆盖参数。

“只读”允许必要会话/审计，禁止修改知识、Agent 或成员。MCP 在执行前拒绝包含写 Wiki、数据库/外部工具、技能、记忆或不明确工具列表的 Agent；quick-answer 和明确只读工具列表可用。

社区工具使用原生建库、文件上传、文档列表/状态和重解析接口。45 项 Python 3.12 测试包含重复上传、重解析、失败重试、建库结果丢失后的重启核对，以及拒绝盲目重发不确定的 POST。原生 Markdown 文件上传/处理另经实际 API 验证；公司社区/S3 端点未连接。

## 5. 模型、配置与迁移

- 固定 Chat/Embedding/Rerank ID 和服务端密钥管理保留；只补缺省项。显式不可用/类型不符模型报错，原生索引、分块及未知合法字段保留；大整数不因 JSON 改写丢失精度。
- Wiki 原生配置不被 Plain RAG 预设覆盖。图谱默认关闭，启用同时要求产品开关、能力配置及原生 Neo4j 依赖；本轮验证缺依赖拒绝，不验证真实图谱抽取质量。
- 可选 VLM 启用时补托管 VLM ID；未配置模型时失败，不接受旧式内联 provider URL/密钥绕开托管策略。真实 PDF/OCR 与可选模型质量待目标环境验收。
- 迁移 000015 新增 `native_session_bindings`、`native_access_events`，不复制历史业务数据。空表 down/up、已有安装记录的拒绝回退/前向恢复，以及非空 R3 绑定/审计拒绝回退均已验证。
- 新运行组合不构造旧 profile、grant、library、publication、subscription 或 note 服务；原生资源操作后旧 profile 表仍为空。历史代码、迁移和数据卷保留。

## 6. 工程检查

| 检查 | 本轮结果 |
| --- | --- |
| `go test -count=1 ./...` | 32 个 Go 包通过，无缓存运行 |
| `go test -race -count=1` | nativeaccess、MCP、enterprise、identity、weknora、server 共 6 包通过 |
| Node 24.19.0 | 隔离 UI 副本的测试、类型检查、生产构建通过；没有 R3 页面改动，原生 UI 功能和四角色截图在 R4 |
| Python 3.12 | 缓存镜像、无外网，导入工具 45 项测试通过 |
| `phase0-check` / `phase5-check` / `stage1-check` | 通过；Phase 5 验证历史组合兼容性，不替代 R3 用例 |
| 路由 / Compose / Shell | 429 条精确清单核对、R3 独立配置及安装脚本语法通过 |
| 设计 / 链接 / 边界 | 22 个中英文编号章节一致，本地链接及 R1/R2 历史证据和 R3 当前指纹通过；上游源码干净 |

没有重复认定 R2 原截图为本轮浏览器验收。R3 未修改页面；当前旧 UI 的业务入口可能收到 410，恢复工作已交接 R4。

## 7. 公开接口限制与修复

[最小提案及代码依据](WORKSPACE_CENTRIC_R3_UPSTREAM_GAPS.md)保留以下边界，不修改上游：

1. 同秒签发的原生 JWT 可能与刚撤销令牌相同：本地密码登录执行完整认证并核对，首次碰到此类失效仅延迟重试一次；不使用宽松认证或刷新绕过。
2. 原生聊天历史向量/混合检索先检索后过滤，整体历史 KB 统计没有产品会话范围：分别返回 409 `history.vector_scope_unavailable`、`history.aggregate_scope_unavailable`；keyword 搜索补入当前有效绑定 session IDs。
3. Agent 内部 `search_conversations` 同样绕过绑定范围，隐式默认工具列表包含该工具：执行前拒绝；保留明确的支持工具配置，不重写原生 Agent。
4. 跨空间相同内置 Agent ID 的来源参数无法在所有公开预检接口中一致解析：返回 409 `agent.source_ambiguous`；普通唯一 ID 的共享 Agent 已通过验证。

另修正了产品身份停用 SQL 的显式参数类型；实际停用/激活接口和拒绝检索已通过。本轮不批准上述上游提案的应用或生产推广。

## 8. R4 与后续验收

[R4 任务清单](WORKSPACE_CENTRIC_R4.md)按基线截图、KB/文档、Agent/模型、对话导航、成员移交、邮箱/CSV 批量成员、退役及浏览器验收拆分。每项包含接口、失败恢复及完成条件。

真实企业 OAuth 浏览器往返与同秒员工注销再登录、真实模型/PDF/OCR/图谱依赖、生产性能/质量、备份恢复和独立网页小助手仍须在对应 R4–R6 环境验收。合成服务只证明已记录的接口行为和权限边界，不代表这些部署条件已通过。
