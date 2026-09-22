# R4 验收记录：原生页面与员工对话

状态：2026-09-16，R4-1 至 R4-7 实现与合成验收完成；尚未部署。R2/R3 原验收文件保留，本轮证据单独记录，不以历史结果替代。

## 实现范围

| 步骤 | 实现及证据 |
| --- | --- |
| R4-1 | [页面/API 依赖表](WORKSPACE_CENTRIC_R4_DEPENDENCIES.md)、[基线](evidence/r4-baseline.json)、四角色 before 截图完成 |
| R4-2 | KB 列表、编辑器、详情、文件、Markdown、FAQ、处理状态、标签、Wiki 与原生模块一致；新 bundle 不包含旧 profile/grant/ingestion API |
| R4-3 | 原生 Agent 列表/编辑/共享；明确原生知识与 Wiki 工具；默认模型保留；关闭扩展和记忆入口，已有不支持配置可显式清除 |
| R4-4 | Viewer 导航与直接链接策略；原生对话请求前解析 scope；关键词历史；角色变化清空选择并刷新；引用和文件继续走员工鉴权 |
| R4-5 | 原生成员/邀请继续使用；平台创建空间；Owner 两步移交，以实际角色恢复，不转移平台身份 |
| R4-6 | 邮箱/CSV 去重与预览；已有成员保持角色；逐项添加、不注册未知员工；失败/结果未知先核对；本地状态仅保存摘要和结果，不保存邮箱及凭证 |
| R4-7 | 新 overlay 边界检查、Node 24/Go/Python 检查、16 个浏览器场景、93 项 API/MCP 检查及四角色前后截图通过；当前源码摘要一致 |

## 接口与恢复

新增 `POST /api/v1/mindcreek/members/preview`：认证人类 Owner，目标空间来自已验证的当前空间和 `X-Tenant-ID`；请求仅接收 `emails`（1–500）。返回该目标空间、逐行 `email/state/user_id/role/code`。状态为 ready、existing、unregistered 或 invalid；不创建账号、不修改成员、不返回全员目录或原生别名。注册冲突和停用账号不能被加入。原生成员列表分页必须完整，否则中止预览。

批量运行重读现有成员，POST 使用原生 `/tenants/:id/members`，从不使用 PUT 改角色。成功项不重发；失败或丢失响应先查询实际成员。新增 Owner 只读预览属于产品接口，不扩展原生 429 路由清单或平台内容权限。

移交在目标成员成为 Owner 后才允许本人降为 Admin，两个独立确认按钮分别触发原生 PUT。刷新后用用户/空间/目标 ID 查询恢复。原生最后 Owner 约束继续生效。写操作不自动重放；原生资源创建失败后需查询资源列表或已知 ID 再决定恢复操作。

浏览器恢复角色时清空旧 Agent/KB/文件/标签选择，并重新加载授权上下文。Viewer 管理链接返回对话；此导航规则不更改后端创建者或共享例外。原生成员移除会撤销该员工已有会话，前端返回企业登录；重新登录后显示受限状态，不自动加回。

## 验证边界

- before 截图使用实际旧 Vue bundle 和模拟 HTTP 响应。
- after 浏览器验证使用真实新 Vue bundle、产品网关和固定原生服务；身份与模型、数据库、文档均为隔离合成数据。员工 token 由合成 OAuth 流程建立，不标记为真实企业浏览器 OAuth 通过。
- 不公开 App/数据库端口；本机浏览器代理转发原始人类凭证和原生响应。没有替换业务 API 响应。
- Markdown 浏览器上传使用原生 Simple 引擎完成解析；原生 DocReader 默认选项保留，合成环境不启动 DocReader。Wiki 原生配置和页面、默认模型及原生文档处理可验证；真实 PDF/OCR、图谱抽取质量及生产依赖仍在后续专项验收。
- R3 历史向量检索、同 ID Agent 来源歧义等边界保持不变；见[接口缺口](WORKSPACE_CENTRIC_R3_UPSTREAM_GAPS.md)。

## 工程检查与证据

2026-09-16 的 [10 组工程检查](evidence/r4-checks.json)通过：Go 1.26 无缓存测试覆盖 32 个包，6 个关键包通过竞态检查；Node 24.19.0 的 594 项前端测试、类型检查和构建通过；Python 3.12 的 45 项导入工具测试通过。原生 429 路由清单、Phase 0/5、stage1、R4 Compose 和安装脚本检查通过。

前端产物及源码摘要见[构建记录](evidence/r4-ui-build.json)。历史页面断言改为验证 `apply-legacy.mjs` / `check-legacy.sh`；默认 overlay 由 R4 原生边界检查验收。历史记录和源码保留，旧页面不进入新 bundle。上游子模块无源码修改。

本轮不新增数据库迁移；独立合成数据库继续验证既有迁移的空库回滚/前滚、有数据时的回滚保护、重启后的安装与会话绑定恢复。完整 API 和浏览器结果分别写入 [R4 API/MCP](evidence/r4-api-probe.json)及[浏览器记录](evidence/r4-browser.json)，两份全量报告及工程检查均通过当前源码摘要校验；93 项 API/MCP 检查涵盖 290 次记录请求，16 个浏览器场景保留 20 张 after 截图。仅清理本轮随机测试项目，R2/R3 历史报告摘要保持不变。

可重复执行：`make r4-executable-checks`、`make r4-api-probe`、`make r4-check`；Node 和浏览器依赖参数见运行说明。

## 页面截图

以下均为合成数据；after 截图等待页面请求和渲染完成。[视觉复核记录](evidence/r4-visual-review.json)保存已检查的四角色及关键面板图片摘要，验证尺寸为 1440 × 1000，不代表移动端或真实企业浏览器验收。

| 角色 | 改动前 | 改动后 |
| --- | --- | --- |
| Owner | [旧页面](../assets/r4/before-owner.png) | [原生知识库](../assets/r4/after-owner.png) |
| Admin | [旧页面](../assets/r4/before-admin.png) | [原生知识库](../assets/r4/after-admin.png) |
| Contributor | [旧页面](../assets/r4/before-contributor.png) | [原生资源入口](../assets/r4/after-contributor.png) |
| Viewer | [旧页面](../assets/r4/before-viewer.png) | [对话入口](../assets/r4/after-viewer.png)、[实际回答](../assets/r4/after-viewer-conversation.png) |

- 原生资源：[创建知识库](../assets/r4/after-create-knowledge-base.png)、[文档上传](../assets/r4/after-native-upload.png)、[FAQ](../assets/r4/after-faq.png)、[Agent 编辑器](../assets/r4/after-agent-editor.png)。
- 成员操作：[批量预览](../assets/r4/after-member-preview.png)、[逐行结果](../assets/r4/after-member-results.png)、[移交中断恢复](../assets/r4/after-transfer-recovery.png)。
- 状态与入口：[平台建空间](../assets/r4/after-platform-create-space.png)、[管理员会话失效](../assets/r4/after-admin-expired.png)、[成员移除后登录](../assets/r4/after-member-removed.png)、[退役书签](../assets/r4/after-retired-link.png)。

## 交接

镜像和隔离运行步骤见 [R4 运行说明](../../deploy/r4/README.md)。R5 继续员工网页小助手，R6 继续目标环境安装、真实身份/模型、备份恢复及生产验收。
