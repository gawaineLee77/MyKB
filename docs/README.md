# MindCreek 文档导航

这里区分**当前可用能力、长期设计、未来开发和历史证据**。文档中的命令默认从仓库根目录执行；文档归档不代表删除功能、关闭待办或批准上线。

## 空间改版入口

新方向与历史实现分开记录：先看[R1 验收](plans/WORKSPACE_CENTRIC_R1.md)、[功能/权限/模块表](plans/WORKSPACE_CENTRIC_R1_MATRIX.md)和[R2 任务](plans/WORKSPACE_CENTRIC_R2.md)。独立本地 admin、默认空间和员工开户已完成 [R2 合成验收](plans/WORKSPACE_CENTRIC_R2_ACCEPTANCE.md)，[新安装手册](../deploy/r2/README.md)提供恢复命令；原生界面已完成 R4 合成验收；员工网页小助手已完成 [R5 合成验收](plans/WORKSPACE_CENTRIC_R5_ACCEPTANCE.md)，企业目标环境验收归 R6。

R3 原生后端、四工具 MCP、退役接口与验证结果见 [R3 验收](plans/WORKSPACE_CENTRIC_R3_ACCEPTANCE.md)。[R3 独立运行配置](../deploy/r3/README.md)用于开发/验收；[R4 清单](plans/WORKSPACE_CENTRIC_R4.md)负责原生页面和 Viewer 导航，结果见 [R4 验收](plans/WORKSPACE_CENTRIC_R4_ACCEPTANCE.md)，运行步骤见 [R4 配置](../deploy/r4/README.md)。

## 从哪里开始

| 目标 | 首选文档 |
|---|---|
| R4 企业内网 AMD64 离线部署与调测 | [AMD64 企业部署手册](guides/R4_AMD64_ENTERPRISE_DEPLOYMENT_ZH.md) / [构建与未通过项](plans/R4_AMD64_PACKAGE_ACCEPTANCE.md) |
| 历史运行组合部署、配置与排障 | [旧组合完整执行手册](guides/FULL_DEPLOYMENT_RUNBOOK_ZH.md) |
| 了解现在实现了什么、有哪些限制 | [当前能力与交付状态](CURRENT_STATUS.md) |
| 了解后续开发顺序和未关闭事项 | [后续开发与待办](ROADMAP.md) |
| 理解架构、权限边界和产品决策 | [总体设计（中文）](OVERALL_DESIGN_ZH.md) / [English](OVERALL_DESIGN.md) |
| 开发与提交代码 | [Repository Guidelines](../AGENTS.md) |

## 当前指导文档

| 主题 | 文档与用途 |
|---|---|
| 部署 | [R4 AMD64 企业内网部署](guides/R4_AMD64_ENTERPRISE_DEPLOYMENT_ZH.md)：当前镜像包安装、OAuth/模型/证书、备份与调测；[完整执行手册](guides/FULL_DEPLOYMENT_RUNBOOK_ZH.md)：当前旧运行组合的部署参考；新空间产品使用 R4 AMD64 手册，目标环境验收归 R6；[构建与局域网](guides/BUILD_AND_LAN_DEPLOYMENT.md)：开发构建和 LAN 补充说明 |
| 知识图谱 | [R4 离线启用与实际使用](guides/R4_KNOWLEDGE_GRAPH_ZH.md)：Neo4j/APOC、原生抽取、问答与停用恢复；[专项验收](plans/R4_KNOWLEDGE_GRAPH_ACCEPTANCE.md) |
| 企业文档链接 | [隐藏上游入口与配置内部地址](guides/R4_ENTERPRISE_LINKS_ZH.md)：运行时开关及 AMD64 前端补丁；[专项验收](plans/R4_UI_LINKS_ACCEPTANCE.md) |
| 日常运维 | [运维与事件处理](guides/PHASE5_OPERATIONS.md)：模型、VLM、上传限制和升级；[全新安装/清理](guides/PHASE5_FRESH_SERVER_INSTALL.md)：仅用于明确放弃旧数据的重装 |
| 身份与密钥 | [企业 OAuth 2.0](guides/PHASE5_IDENTITY_PROVIDER.md)、[密钥生命周期](guides/PHASE5_SECRETS.md) |
| 可靠性与上线 | [备份恢复](guides/PHASE5_BACKUP_RECOVERY.md)、[观测与告警](guides/PHASE5_OBSERVABILITY.md)、[受控试点](guides/PHASE5_PILOT.md) |
| 智能体接入 | [问答与 MCP 使用说明](guides/AGENT_AND_MCP.md) |
| 员工网页小助手 | [R5 启用、发布与排障](guides/R5_EMPLOYEE_ASSISTANT_ZH.md)：Owner/Admin 发布渠道，员工以企业身份在其他网页提问 |
| 社区资料导入 | [帖子、S3 图片与附件导入](guides/COMMUNITY_IMPORT.md)：API 配置、分帖归档、上传、重试与更新 |
| 镜像与上游 | [镜像构建和离线归档](../images/README.md)、[v0.8.0 升级记录](upgrades/WEKNORA_V0.8.0_UPGRADE.md)、[上游补丁准入与台账](UPSTREAM_PATCHES.md) |

## 未来功能设计

- [后续开发与待办](ROADMAP.md)：R6 企业交付、已取代的笔记/订阅事项、后置扩展和目标环境验收。
- [R5、R6 收尾后的七项功能待办](plans/POST_R6_FEATURE_BACKLOG_ZH.md)：图谱展示与人工修订、知识版本、自动清洗、Wiki 下载、定时处理和聊天按钮配置；当前仅登记，主线收尾后再分步实现。
- [本地桌面端（中文）](plans/DESKTOP_LOCAL_DESIGN_ZH.md) / [English](plans/DESKTOP_LOCAL_DESIGN.md)：本地解析、存储、检索和 MCP，使用内部在线模型服务。
- [智能体发布与企业渠道](plans/AGENT_PUBLISHING_CHANNELS_DESIGN_ZH.md)：历史候选渠道设计；员工网页部分以新 R5 为准，小程序等不在本次范围。

## 技术参考与历史记录

- `reference/`：持续维护的[网关边界](reference/PHASE1_GATEWAY.md)、[路由策略](reference/PHASE1_ROUTE_POLICY.md)、[分享模型](reference/PHASE2_SHARING_MODEL.md)、[路由动作](reference/PHASE2_ROUTE_ACTIONS.md)；另保留注明日期的[WeKnora/Dify 选型比较](reference/WEKNORA_VS_DIFY.md)。
- `upgrades/`：仍与当前部署相关的版本兼容、迁移和回滚记录。
- [archive/](archive/README.md)：Phase 0–5 的任务计划、Gate 验收、旧升级流程与截图。仅用于追溯，不作为当前部署步骤。
- `assets/`：保留旧架构图片；新版总体设计使用 Mermaid，历史图片不作为新架构证据。

## 文档维护约定

1. 当前事实写入 `CURRENT_STATUS.md`；未来范围与未关闭事项写入 `ROADMAP.md`，详细方案放在 `plans/`。
2. 架构约束以总体设计和补丁台账为准；实际接口以当前配置、适配器和测试为准，不把设计中的建议当成已上线功能。
3. 完成的阶段过程记录归档，但先登记剩余事项；不补勾历史任务，不把旧测试结果冒充新版本或目标服务器的验收。
4. 部署手册是完整操作入口，专题指南维护各自细节。文件名中的 Phase 号保留用于追溯，不要求用户按旧阶段逐次部署。
5. 移动文档时同步修改相对链接、图片和验证脚本。原根目录文件现按同名分类保存；历史记录的日期、基线和证据不重写。

- R5：[实施清单](plans/WORKSPACE_CENTRIC_R5.md)、[验收记录](plans/WORKSPACE_CENTRIC_R5_ACCEPTANCE.md)、[员工网页小助手使用指南](guides/R5_EMPLOYEE_ASSISTANT_ZH.md)。
