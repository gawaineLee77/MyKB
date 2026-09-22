# MindCreek 当前能力与交付状态

2026-09-18 增量：企业文档链接采用默认隐藏、按项配置内部地址的运行时策略；需安装一次前端 `links1` 补丁，之后改配置并刷新即可生效。见[配置与部署指南](guides/R4_ENTERPRISE_LINKS_ZH.md)和[验收记录](plans/R4_UI_LINKS_ACCEPTANCE.md)。企业目标服务器尚未应用此补丁。

更新日期：2026-09-22。当前进入企业部署调测阶段。R1 完成新设计；R2 已实现并完成合成验收。R3 已实现独立原生后端，验收见下；R4 页面实现与合成验收已完成；R5 实现与合成验收已完成；R6 待企业目标环境验收。历史运行组合仍保留。

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
- **首次安装优先：**没有现网用户数据迁移任务；现有本地开发数据和容器保持原状。真实企业 OAuth、模型、浏览器嵌入及生产验收留在后续对应阶段。

## 产品基线

- 已导出的企业调测包基线为 R4；R5 源码与合成验收已交付，需另行构建并验证目标镜像；Phase 5 保留为历史运行组合；当前仓库批准的上游为未修改的 WeKnora **v0.8.0**，详见[升级记录](upgrades/WEKNORA_V0.8.0_UPGRADE.md)。
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
