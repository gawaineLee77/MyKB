# R3 路由与权限接管清单

## 固定路由

[机器可执行清单](../../config/r3-routes.json) 包含固定 WeKnora v0.8.0 的 429 个 method/path、处理器、旧分类及新产品校验点：280 个原生入口，149 个既有关闭入口。由只读 Go overlay 调用真实路由注册函数生成；未登记的方法或路径一律拒绝。

| 接口族 | 授权接管 | 产品附加检查 | 阶段 |
| --- | --- | --- | --- |
| knowledge-bases、knowledge、chunks、FAQ、文件、tags | 原生 RBAC、创建者及 KB 共享 | 保留原请求/响应，原生写授权；上传与模型配置 | R3-2/4 |
| knowledgebase/:kb_id/wiki | 原生 KB 权限 | 原生 Wiki 开启条件；无需 product profile | R3-2/4 |
| agents、shared-agents、共享、收藏、organizations | 原生 Agent/组织/资源权限 | 服务端 Agent 知识绑定检查 | R3-2/3 |
| knowledge-search、knowledge-chat、agent-chat | 原生调用能力与资源范围 | 检索前显式范围全量检查，Agent 服务端范围，会话绑定 | R3-3 |
| sessions、messages、消息文件与引用 | 原生会话权限 | 产品主体类型/ID、空间、历史 KB 范围及撤销重查 | R3-3 |
| models、initialization | 原生模型可见性及管理权限 | 托管缺省值、合法显式模型、保留原生参数 | R3-4 |
| auth、tenants、members、invitations、system/admin | 原生认证及平台/Owner 权限 | R2 企业登录、开户、固定 admin 与受保护安装设置 | 保留 R2 |
| 原生公有 embed、原文件能力链接、IM、外部连接器等 | 保持关闭 | 不通过放开渠道令牌模拟员工身份 | R5 或另行设计 |

## R4 图谱增量

2026-09-16：清单仍为 429 条，三个 `POST /initialization/extract/{fabri-tag,fabri-text,text-relation}` 由旧 disabled 分类改为原生处理 + `models_upload` 校验。`baseline_rule` 保留历史来源。图谱开关关闭或 app 未报告 Neo4j 时返回 409 `graph.dependencies_unavailable`；有效 Chat 模型与原生 Owner/Admin/API Key 能力继续校验。默认能力文件仍关闭图谱，只有显式部署补充包才开启。其余路由不变，历史 R3 报告不重写。

## 退役产品接口

下列产品命名空间任意方法返回 HTTP 410、`feature.retired`，不能落入原生代理；原生接口不受同名业务概念影响。

| 旧接口 | R3 替代 |
| --- | --- |
| /api/v1/knowledge-spaces | 原生 POST /api/v1/knowledge-bases |
| /api/v1/knowledge-bases/:id/product-profile | 原生 KB 配置 |
| /api/v1/knowledge-bases/:id/notes/** | 退役；原生 manual、Markdown、FAQ、Wiki 保留 |
| /api/v1/knowledge-bases/:id/ingestions/** | 原生 knowledge/file、url、manual、处理状态及重试 |
| /api/v1/mindcreek/knowledge-bases/** | 原生资源列表、共享与访问检查 |
| /api/v1/mindcreek/users | 原生成员与企业身份目录 |
| /api/v1/mindcreek/catalog、publications/**、me/subscriptions | 退役，无发布订阅兼容层 |

原有 `/mindcreek/agent/scope` 及 resolve 接口已改用原生范围，供 R4 过渡接入；不查询 library/grant/profile。MCP 仅四工具，移除工具按标准未知工具错误处理。

## 四角色检查依据

| 行为 | Owner/Admin | Contributor | Viewer | 依据 |
| --- | --- | --- | --- | --- |
| KB/Agent 创建 | 原生允许 | 原生允许 | 原生拒绝 | routes_knowledge.go、routes_agent.go |
| KB 内容/Agent 编辑删除 | 原生管理权限 | 创建者与对应 KB 写权限 | 按上游创建者例外，不人为抹除 | middleware/rbac.go、routes_knowledge.go |
| 读取/预览 | 有效资源权限 | 有效资源权限 | 有效资源权限 | KBAccessRead |
| 原文件下载 | Contributor+ 且 KB 写权限 | 同左 | 拒绝 | routes_knowledge.go DownloadKnowledge |
| 成员修改/所有权移交 | 仅 Owner | 拒绝 | 拒绝 | routes_auth_tenant.go |

本轮允许/拒绝与恢复结果见[R3 验收](WORKSPACE_CENTRIC_R3_ACCEPTANCE.md#3-权限与范围证据)，每项请求记录在[合成报告](evidence/r3-api-probe.json)。平台管理员不会绕过空间内容权限。机器不采用四角色推断，而由原生 API Key capability/KB scope 检查。
