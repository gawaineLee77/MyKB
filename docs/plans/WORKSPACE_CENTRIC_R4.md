# R4：原生页面恢复与员工对话入口

状态：2026-09-16 已完成 R4-1 至 R4-7，实现与合成验收结果集中于 [R4 验收记录](WORKSPACE_CENTRIC_R4_ACCEPTANCE.md)。依赖 [R3 后端验收](WORKSPACE_CENTRIC_R3_ACCEPTANCE.md)；本清单保留实施顺序；尚未部署，目标环境验收归 R6。上游源码保持干净。

## 固定边界

- 保留 `/` 企业登录/恢复、`/admin/login`、开户及安装状态页、默认模型和 R2 会话维护。
- Viewer 仅展示对话、历史、必要账户操作及对话内空间/Agent/KB 选择；菜单收敛不改变原生后端权限。
- Contributor 保留原生创建和管理自有资源的能力；Owner/Admin 使用原生管理页面。成员变更与移交仍要求 Owner。
- 平台管理员身份只决定平台操作，不授予所有空间内容访问权。
- 网页小助手在 R5；不开放匿名 Embed、外部连接器、技能沙箱或记忆入口。

## 分步任务

| 步骤 | 输入与改动 | 完成输出 / 验收 |
| --- | --- | --- |
| R4-1 原生 UI 基线 | 固定 v0.8.0 frontend 与当前 overlay；列出替换路由、旧 API、菜单、组件；四角色截图 | 页面/API 依赖表及 before 截图；保留登录、开户、品牌、模型适配 |
| R4-2 KB 与文档 | 恢复原生 KB 列表、创建、详情、手工文档、文件、FAQ、处理状态、标签与设置；撤去旧 profile/grant/ingestion 路径 | 走原生 `/knowledge-bases`、`/knowledge`、`/chunks`；上传、重试、预览/下载、FAQ 权限与原生一致 |
| R4-3 Agent 与模型 | 恢复 Agent 创建、编辑、共享和选择；缺省模型由服务器补充；原生 Wiki/索引设置保留 | 使用 `/agents`、`/shared-agents` 和原生共享接口；提供明确支持的工具列表；无员工密钥输入要求 |
| R4-4 对话与导航 | 接通原生检索/会话和 R3 scope；实现 Viewer 导航与直接链接策略；切换空间、刷新和角色变化时重算 | 显式越权 KB 整体报错；空范围明确提示；历史、引用、文件和重连撤销后拒绝；Contributor 不被降为 Viewer |
| R4-5 成员与移交 | 接通 R2 原生成员/邀请接口、企业邮箱映射；平台管理员建空间；Owner 两步移交 | 预览当前及目标 Owner；中断后重新查询角色恢复；最后 Owner 保护；移交不转移平台身份 |
| R4-6 邮箱/CSV 批量成员 | 解析和去重已有员工邮箱；预览目标空间、固定角色和逐行结果；复用原生逐项接口 | 不注册未知账户、不覆盖已有高角色；重复执行只重试失败项；报告未开户/无权/冲突；部门目录同步后置 |
| R4-7 退役与验收 | 移除目录、发布订阅、个人笔记及旧链接；保留原生 Markdown、FAQ、Wiki；更新 overlay 检查 | Node 24 测试、类型检查、构建；四角色 before/after 截图及真实浏览器合成 API 联调；R2/R3 回归；无上游源码修改 |

## 接口与错误交接

| 页面用途 | 后端契约 / 处理要求 |
| --- | --- |
| 身份与空间 | `/api/v1/auth/me`、`/tenants`、`/mindcreek/onboarding`；无空间仅开户/恢复；被移除不自动加回 |
| KB / Agent 写操作 | 保留原生请求、响应和错误；创建者例外由对应写接口决定；禁止以可读推导可写 |
| 默认 / Agent 范围 | `GET /mindcreek/agent/scope`；`POST .../resolve` 接收 `selection: default/explicit`、`knowledge_base_ids`、`agent_id`；返回实际 KB IDs，最大 256 |
| 共享 Agent | `/agents/:id` 是本空间详情，跨空间原生返回 404；用 `/shared-agents` 的 `agent` 配置和 `source_tenant_id` 展示共享项；R3 scope 同样从授权列表解析 |
| 引用、图片与文件 | 使用带员工会话的知识/会话资源接口；原生公有 `/files`、presigned 能力链接保持关闭，不能用它们替代员工鉴权；KB 上传仍沿用现有文档类型策略，图片/OCR 按可选 VLM 条件验收 |
| 会话列表 | `/sessions` 返回仅属于当前主体且授权仍有效的绑定会话和分页总数；旧会话不接纳 |
| 历史搜索 | `/messages/search` 显式发送 `mode: keyword`；409 `history.vector_scope_unavailable` 不降级为无范围向量检索；403 `session.scope_empty` 显示无可搜索历史 |
| Agent 工具限制 | 409 `agent.tools_scope_unavailable`：编辑为明确工具列表；`search_conversations` 受历史范围缺口限制；MCP 额外拒绝写 Wiki、数据库/外部工具、技能和记忆 |
| Agent 同 ID 来源歧义 | 409 `agent.source_ambiguous`：不可默默改选其他空间 Agent；使用唯一自定义 Agent，补丁方案另行架构审查 |
| 图谱 / VLM | 409 `graph.dependencies_unavailable` 显示依赖未就绪；VLM 可选模型缺失报错；不改写 Wiki 或原生索引参数 |
| 退役 / 未知 | 410 `feature.retired` 指向原生入口；未知路由保持拒绝，不自动尝试旧代理 |

原生字段以[精确路由清单](../../config/r3-routes.json)和固定上游为准。R3 的文档上传、处理、检索与模型证据是合成环境结果；真实 PDF/OCR、图谱抽取、企业浏览器策略及生产质量继续在对应阶段验收。

## 失败恢复与交付

页面写入结果不明时先按原生 ID 查询；禁止自动重发建库、建空间、邀请或所有权移交。成员批量处理保存非敏感逐行状态，不保存 bearer、Key 或密码。角色/空间变化后清空旧选择并重新拉取授权范围；401 按原登录方式返回员工登录或 `/admin/login`。

最终交付页面依赖表、四角色截图、可执行浏览器证据和新 UI 镜像构建说明。R3 历史向量/Agent 歧义缺口按[提案](WORKSPACE_CENTRIC_R3_UPSTREAM_GAPS.md)单独处理，不通过放宽权限解决。
