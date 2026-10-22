# MindCreek 当前能力与交付状态

2026-10-22 问答链接源码修复（尚未封包）：普通回答、智能体回答和 Wiki 正文中的网页链接保留新标签页打开属性，避免替换当前问答页面。修复 DOMPurify 的 `USE_PROFILES` 覆盖属性白名单后移除 `target` 的问题；引用抽屉、图片和附件预览沿用原交互，链接仍保留 `noopener noreferrer` 和原有 HTML 安全过滤。646 项前端测试、类型检查、构建及产品边界检查通过；真实浏览器 20 项检查及 14 张前后截图验证完成，确认会话和草稿保留。证据见[本轮记录](plans/evidence/chat-links-20261022.json)。未构建升级包或部署。

2026-10-22 源码修复（尚未封包）：回答导出 Word 新增三种单线箭头的上下标注转换，修复 `\xrightarrow{\text{…}}` 导出时的 `mpadded` 错误。646 项前端测试、类型检查、构建、产品边界及新增浏览器公式结构回归通过；主复现公式已在 Word 中确认显示正确。按用户要求，以导出正确、界面无报错验收，不再尝试本机编辑保存。LibreOffice 预览存在独立导入兼容问题。见[使用说明](guides/CHAT_WORD_EXPORT_ZH.md#带文字箭头的-mpadded-报错)和[本轮记录](plans/CHAT_WORD_EXPORT_ACCEPTANCE.md#2026-10-22-带标注箭头源码修复)。本轮未构建升级包或部署。

2026-10-21 源码修复（尚未封包）：知识库页面的窗口返回刷新已统一协调，过期列表或权限响应不能覆盖新结果；加载／权限核验中显示等待，接口失败显示重试，只有确认无可见知识库时才显示空态。Owner、Admin、Contributor 列表及 Viewer 问答选择／撤权已完成合成浏览器回归；646 项前端测试、类型检查、构建、10 项缓存测试及产品边界检查通过。见[修复说明及证据](guides/MODEL_REFRESH_FIX_ZH.md#2026-10-21-知识库列表后续修复未封包)。本轮未构建镜像、升级包或部署；以下已封存的 20261020 包不包含此项修复。

2026-10-20 合并修复完整包：`current-20261020-699031ce-104658` 在此前产品能力基础上，修复窗口／标签页返回后模型与智能体误报未配置，以及历史知识库／问答图片权限校验。图片已有可信解析关联时自动恢复；已登记但缺少关联时通过随包工具人工核验，缺失登记或文件不自动重建。产品迁移为 `000022`，支持 R4 图谱版、1009、1019 直接升级；不必先安装当日较早图片包。见[合并修复说明](guides/MODEL_REFRESH_FIX_ZH.md)及[本次验收](plans/CURRENT_AMD64_MODEL_FIX_20261020.md)。未部署企业服务器，目标环境验收仍待执行。

2026-10-19 历史完整包：`current-20261019-d3fafdaf` 纳入下述空间管理、知识库权限、全站外观、智能体图标和 Wiki MCP 增量，以及已有 R5、图谱浏览和 Word 导出。面向 R4 + 图谱补丁与 1009 两种 Linux AMD64 离线部署，均支持直接升级，无需旧 Phase 5。交付验证见[本轮包验收](plans/CURRENT_AMD64_UPGRADE_20261019.md)，操作见[两类实例升级指南](guides/UPGRADE_R4_AND_20261009_ZH.md)。企业服务器尚未升级；目标物理 AMD64 数据库检查和业务验收须在维护窗口执行。

2026-10-19 增量：[服务外观、智能体图标与 Wiki MCP](guides/SERVICE_APPEARANCE_AND_WIKI_MCP_ZH.md)已在产品层实现。安装管理员可配置全站名称/Logo（初始“小纳”）；知识库来源筛选五项；自定义智能体支持图片图标；对外 MCP 新增 Wiki 搜索与正文读取。新增 000020–000021 产品迁移，保留上游源码不变。验证及限制见[验收记录](plans/SERVICE_APPEARANCE_AND_WIKI_MCP_ACCEPTANCE.md)。现已纳入上述完整包。

2026-10-18 增量：[知识库细粒度权限](guides/KNOWLEDGE_BASE_PERMISSIONS_ZH.md)已在产品网关和前端覆盖实现：默认私有、空间公开/指定成员、七项协作操作、Owner/Admin 全库管理、机器显式范围及持久撤销。新增 000018–000019 产品迁移和独立协作执行授权；同一规则用于 Web、问答、MCP、小助手、图谱和历史访问。无法证明归属的派生资源与不受控 Agent 工具保持关闭，详见[本轮验收与限制](plans/KNOWLEDGE_BASE_ACL_ACCEPTANCE.md)。现已纳入上述完整包。

2026-10-16 增量：[空间权限扩展](guides/WORKSPACE_ADMIN_PERMISSIONS_ZH.md)支持 Admin 管理非 Owner 成员、批量导入、空间名称/描述和自己的 API Keys；Owner 可管理全部普通 Keys。采用产品网关和前端覆盖，未修改上游。新增 000017 产品迁移、安装加密密钥及逐空间 Owner 授权；测试证据与部署边界见[验收记录](plans/WORKSPACE_ADMIN_ACCEPTANCE.md)。现已纳入上述完整包。

2026-10-09 历史包：纳入 R5、知识图谱只读展示和 Word 导出，提供全新安装、v0.8.0 企业包保留数据升级、冷备份及隔离恢复工具。见[新部署指南](guides/ENTERPRISE_DEPLOYMENT_AND_UPGRADE_ZH.md)和[当时包验收](plans/CURRENT_AMD64_PACKAGE_ACCEPTANCE.md)。该包与历史 R4 包/补丁均保留，作为本轮升级来源和原版回退依据。

2026-10-08 增量：[单条回答 Word 导出](guides/CHAT_WORD_EXPORT_ZH.md)已接入主站普通问答及智能体回答，四角色可导出已显示内容；表格与支持的公式保持原生可编辑。不支持的公式明确报错，不以图片降级。见[验收](plans/CHAT_WORD_EXPORT_ACCEPTANCE.md)。只需更新前端，无新增后端服务；本轮未构建镜像或部署。

2026-10-08 增量：E-01 [知识图谱只读浏览](guides/KNOWLEDGE_GRAPH_BROWSER_ZH.md)已实现，Owner/Admin/Contributor 可查看有权访问的所属空间知识库图谱，Viewer 不可用。新增产品网关读取接口及前端页签，见[本轮验收](plans/KNOWLEDGE_GRAPH_BROWSER_ACCEPTANCE.md)。需升级网关/前端并配置私有 Neo4j 连接；尚未构建新离线镜像或部署企业服务器。

2026-09-18 增量：企业文档链接采用默认隐藏、按项配置内部地址的运行时策略；需安装一次前端 `links1` 补丁，之后改配置并刷新即可生效。见[配置与部署指南](guides/R4_ENTERPRISE_LINKS_ZH.md)和[验收记录](plans/R4_UI_LINKS_ACCEPTANCE.md)。企业目标服务器尚未应用此补丁。

更新日期：2026-10-22。当前进入企业部署调测阶段。R1 完成新设计；R2 已实现并完成合成验收。R3 已实现独立原生后端，验收见下；R4 页面实现与合成验收已完成；R5 实现与合成验收已完成；R6 待企业目标环境验收。历史运行组合仍保留。

## 空间改版进度

- **R1 已交付：**[验收记录](plans/WORKSPACE_CENTRIC_R1.md)、[三张对照表](plans/WORKSPACE_CENTRIC_R1_MATRIX.md)、原生接口合成验证及[R2 任务清单](plans/WORKSPACE_CENTRIC_R2.md)。
- **目标已确定：**独立本地 admin、公司默认空间、员工 OAuth2 首次加入 Viewer；原生四角色及 KB/Agent 页面；取消发布订阅与个人笔记；保留托管模型和企业身份；员工专用网页小助手。
- **R2 已交付：**[验收记录](plans/WORKSPACE_CENTRIC_R2_ACCEPTANCE.md)。`/admin/login`、安装/开户记录、默认空间、首次 Viewer、平台建空间及原生成员 API；9 个模拟 API 浏览器用例与前后截图。使用[新安装配置](../deploy/r2/README.md)开启，历史配置不自动切换。
- **R3 后端已实现：**[验收记录](plans/WORKSPACE_CENTRIC_R3_ACCEPTANCE.md)、[路由/权限表](plans/WORKSPACE_CENTRIC_R3_ROUTES.md)；独立原生授权、主体会话绑定、原生模型配置、旧接口 410、四工具 MCP 和原生社区导入。
- **R4 已交付合成验收：**[原生 UI、Viewer 导航和批量成员](plans/WORKSPACE_CENTRIC_R4_ACCEPTANCE.md)。R5 员工小助手已交付合成验收，本轮证据见 [R5 验收记录](plans/WORKSPACE_CENTRIC_R5_ACCEPTANCE.md)；R6 目标环境安装验收待完成。
- **原生图谱增量已通过合成验收：**[离线补充包与实际使用](guides/R4_KNOWLEDGE_GRAPH_ZH.md)、[专项记录](plans/R4_KNOWLEDGE_GRAPH_ACCEPTANCE.md)。Neo4j/APOC 实体关系入库、原生预览、RAG/快速问答 Agent 与 MCP 引用、权限隔离和恢复已验证；保留登录修复。普通 Agent 图谱工具仍有上游限制，企业模型质量与物理 AMD64 验收另行进行。
- **AMD64 企业调测包：**八个 AMD64 运行镜像、独立企业 Compose、安装工具、TLS/CA 与[详细部署手册](guides/R4_AMD64_ENTERPRISE_DEPLOYMENT_ZH.md)。本机 AMD64 模拟的 BM25 查询异常退出，目标物理 AMD64 必须先运行包内数据库预检；不宣称混合检索或 R6 生产验收通过。
- **企业 Client Secret 修复：**取消外部 OAuth2/OIDC 凭证的 16 字符最小长度，保留非空检查；原 AMD64 完整包使用[独立网关补丁](guides/R4_OAUTH_CLIENT_SECRET_HOTFIX_ZH.md)。历史 R4 全量证据对应修复前指纹；补丁单独记录针对性验证。
- **员工登录诊断：**[调用链日志补丁与 Phase 5 配置对照](guides/R4_IDENTITY_DIAGNOSTICS_ZH.md)区分 state/Cookie/事务问题和 TLS/接口错误，保留短密钥修复。当前企业模板与旧 Phase 5 的请求选项不同；R5 前端已补齐首页中转的 state 转发，并完成合成浏览器往返验证；历史网关日志补丁本身不包含此前端修复。
- **安装与升级：**新包支持已有 v0.8.0 原生空间企业实例的受控升级；不迁入旧 Phase 5 发布订阅/笔记数据。本地历史开发数据和容器保持原状。真实企业 OAuth、模型、浏览器嵌入及生产验收留在后续对应阶段。

## 产品基线

- 当前完整企业包为 `current-20261020-699031ce-104658`，包含 R5、图谱浏览、Word 导出、空间管理、知识库权限、全站外观、智能体图片和 Wiki MCP，并合并历史图片权限与模型缓存修复（产品迁移 `000022`）；原 R4、1009、1019 及当日较早图片包作为历史交付／回退依据保留；Phase 5 保留为历史运行组合；当前仓库批准的上游为未修改的 WeKnora **v0.8.0**，详见[升级记录](upgrades/WEKNORA_V0.8.0_UPGRADE.md)。
- Phase 0、Phase 1 Gates A–D 和 Phase 2–5 的工程记录已归档。Phase 1 可选/收尾任务、Phase 5 运维验收不能据此自动视为完成，见[待办](ROADMAP.md)。
- WeKnora 负责文档解析、分块、索引、检索、重排与智能体执行；R3 MindCreek 提供原生权限前的身份/范围校验、模型和身份接入、产品 UI 与部署封装，不另造一套 Plain RAG 引擎。
- 浏览器与 MCP 通过产品入口访问；网关、App 和存储组件不直接对外开放。

## 历史运行组合的能力及边界

| 能力 | 当前行为 | 边界 |
|---|---|---|
| Personal Notes | 创建、编辑、TXT/Markdown 导入、修订与恢复 | 仅所有者使用；不分享、不发布；保留小文件配额 |
| Document RAG | 多格式导入、处理状态、重试/重解析、删除、检索与引用 | 使用 Plain RAG；最大上传限额可配置为 1–500 MiB，不改变个人笔记配额 |
| 原生“问答”知识库 | 产品创建入口复用 WeKnora FAQ 类型，可供智能体检索 | 导入格式按上游 FAQ 模板；更新代码不等于服务器已加载新 UI 镜像 |
| 分享与组织公开 | 私有默认、显式 Viewer/Editor、受控组织公开 | Personal Notes 除外；不能因普通工作空间成员身份读取私有库 |
| 发布与订阅 | 内部目录、发布/撤销、实时订阅、更新提示 | 订阅是引用，不复制内容；撤销后需重新鉴权 |
| Web 智能体 | 授权知识范围检索、回答、引用；显式无知识库智能体可纯聊天 | 公开但未订阅的 KB 需显式选择；纯聊天不获得文档访问权限 |
| MCP | 6 个只读工具，共享 Web 的知识权限边界 | 必须认证；不是匿名服务，也不提供写入工具或企业小程序发布入口 |
| 托管模型 | 默认 Chat、Embedding、Rerank；可选 VLM/OCR 与连接检查 | 密钥由管理员配置；普通用户自定义模型默认关闭；已有库更换 Embedding 需重建索引 |
| 企业身份 | plain OAuth 2.0、可选 OIDC、首次登录建号、关闭公开注册 | 真实 OAuth、证书、代理和员工策略由目标环境配置验证 |
| 运维保障 | TLS、备份/恢复、观测、迁移、探针和试点工具 | 合成测试通过不替代生产验收；破坏性清理和外部扫描仍受批准约束 |

上表仅描述历史运行组合；发布、订阅、个人笔记和旧 grant/profile 在 R3 独立组合中不再注册。历史开关以 [Phase 5 capabilities](../config/phase5-capabilities.json) 为准；路由边界见[策略参考](reference/PHASE1_ROUTE_POLICY.md)。扫描 PDF 的 VLM 文字提取不等于 PixelRAG；上游包含 Graph/Wiki 功能也不代表 MindCreek 已开放相应产品工作流。

## 尚未作为当前产品开放

- 原生 Wiki 配置/API 与 R4 页面已接通；图谱默认关闭，Neo4j 与配置依赖就绪后才能启用。真实抽取质量及自研 GraphRAG、PixelRAG、本体后置。
- 原生历史向量/混合搜索、跨来源同 ID Agent 歧义受[公开接口缺口](plans/WORKSPACE_CENTRIC_R3_UPSTREAM_GAPS.md)限制；安全路径和明确错误见 R3 验收。
- 旧 Note Wiki 未完成事项随笔记功能退役而被取代，不标为开发完成。
- 桌面本地版、独立门户和小程序仍为设计；员工网页小助手已由 R5 交付，默认关闭，启用见[操作指南](guides/R5_EMPLOYEE_ASSISTANT_ZH.md)。
- IM、CLI、小程序、公开 Embed、Web 搜索等排除能力保持关闭；用户自带模型仅是受控可选项，不因文档整理而启用。

## 验收与部署如何阅读

另提供独立的[社区帖子导入工具](guides/COMMUNITY_IMPORT.md)：支持 HTML/S3 图片、文档主文件、Markdown、附件、分帖版本和原生建库/上传/重解析接口。公司 API 与下载端点由操作员填写；不是已部署的自动同步连接器，不自动同步来源 ACL 或删除事件。

该工具已于 2026-09-11 完成工程收尾：Python 3.12/3.14 各 41 项合成测试及网关测试通过，已验证跨进程续传与重复执行。配置完成后的真实接口、解析/OCR 和问答质量仍需样本联调，详见[交付验证](guides/COMMUNITY_IMPORT.md#9-工程交付验证)。

[历史验收](archive/README.md)记录的是各阶段当时的环境和版本。Phase 5 曾记录外部漏洞扫描结果、真实身份登录、团队评价、费用和目标服务器恢复等待补证据；本次整理不访问该服务器，也不替这些事项签字。

[现有执行手册](guides/FULL_DEPLOYMENT_RUNBOOK_ZH.md)仍对应旧 Phase 5 运行组合，不能用于宣称新空间产品已可部署。R3 独立安装/恢复入口见[运行配置](../deploy/r3/README.md)；R4 UI 和验收步骤见[R4 运行说明](../deploy/r4/README.md)，[R4 AMD64 企业部署手册](guides/R4_AMD64_ENTERPRISE_DEPLOYMENT_ZH.md)提供新的离线调测入口；目标环境验收继续后置，无需重走 Phase 0–4。
