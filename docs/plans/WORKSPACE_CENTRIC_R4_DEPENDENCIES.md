# R4 页面与 API 依赖表

基线：WeKnora v0.8.0 / `1edcd54b43606d9079bb36650efe3f68707a79ea`。R3 历史证据摘要见 [r4-baseline.json](evidence/r4-baseline.json)；保留开始前全部未提交工作。以下路径均相对仓库根目录。

| 页面 / 模块 | 原生代码依据 | R4 处理与 API |
| --- | --- | --- |
| KB 列表 | `upstream/weknora/frontend/src/views/knowledge/KnowledgeBaseList.vue` | 恢复原生组件；`/knowledge-bases`、`/shared-knowledge-bases`；不再调用 library/profile/grant |
| KB 创建、文档、FAQ、Wiki | `src/views/knowledge/KnowledgeBaseEditorModal.vue`、`src/views/knowledge/`、`src/api/knowledge-base/index.ts`（均在 upstream frontend） | 保留原生创建者/共享判断；原生 `/knowledge`、`/chunks`、`/knowledge-bases/:id`；默认模型由网关补缺省值 |
| 原生实体关系图谱（R4 增量） | `src/views/knowledge/settings/GraphSettings.vue`、原生 `initialization/extract/*` | [补充包](../guides/R4_KNOWLEDGE_GRAPH_ZH.md)启用真实 Neo4j/APOC，Owner/Admin 预览，原生配置保存；普通 Agent 图谱工具与问答流水线的区别见专项验收 |
| Agent | `src/views/agent/AgentEditorModal.vue`、`AgentList.vue`、`src/stores/organization.ts` | 保留原生编辑/共享；共享列表含 Agent 配置和来源空间；工具显式选择，隐藏已排除扩展与记忆 |
| 对话 / 历史 | `src/api/chat/streame.ts`、`src/api/chat-history.ts` | 问答前调用 R3 scope；历史强制关键词；会话、引用和文件保留真实员工凭证 |
| 导航 / 账户 | `src/stores/menu.ts`、`src/components/UserMenu.vue`、`src/views/settings/Settings.vue` | Viewer 对话与必要账户操作；Contributor 原生能力不变；导航与硬刷新重新读取成员信息 |
| 企业身份 | `tools/frontend-overlay/product/mindcreek/enterprise-entry.ts` 等 R2 模块 | 保留企业自动登录、首次开户、admin 登录及安装状态 |
| 成员 / 移交 | `src/views/settings/TenantMembers.vue`、`src/api/tenant/{members,invitations}.ts` | 原生成员与邀请；Owner 两步移交；结果不明先查询实际角色 |
| 批量员工 | 产品 `EnterpriseMembers.vue` | 邮箱/CSV 预览、去重；逐项原生添加，保留高角色，失败核对后重试；不开户未知员工 |
| 退役 UI | `tools/frontend-overlay/product/mindcreek/` 下 KnowledgeLibrary、Notes、RAG、Catalog、Publication、Sharing、Ask 及旧 API | 历史源码保留；新 bundle 不复制、不路由到这些组件；旧书签进入退役提示 |

## 四角色改动前截图

截图运行实际旧 Vue bundle，接口由合成响应提供，不表示真实后端或企业 OAuth 联调通过。

- [Owner](../assets/r4/before-owner.png)
- [Admin](../assets/r4/before-admin.png)
- [Contributor](../assets/r4/before-contributor.png)
- [Viewer](../assets/r4/before-viewer.png)

四角色旧页面均显示旧目录入口，Viewer 仍可见 KB/Agent 菜单。[执行记录](evidence/r4-before.json)。

Agent 列表的创建动作等待原生模型列表加载完成，避免托管模型尚在加载时误判缺失。原生 Agent 列表布局与资源 API 保持；编辑器只增加支持工具及记忆边界校验。
