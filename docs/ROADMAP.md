# MindCreek 后续开发与未关闭事项

更新日期：2026-09-22。主线已按空间改版调整。R1 设计基线和 R2 登录/开户已交付合成验收；R3 后端实现与合成验收已完成；R4 实现与合成验收已完成；R5 实现与合成验收已完成；R6 待企业目标环境验收。设计以首次企业安装为基线，不安排旧业务数据迁移；当前已进入企业调测，R6 需记录目标环境实际结果。

## 当前主线：R1–R6

| 阶段 | 状态及交付 | 验收重点 |
|---|---|---|
| R1 | 已交付：[记录](plans/WORKSPACE_CENTRIC_R1.md)、[对照表](plans/WORKSPACE_CENTRIC_R1_MATRIX.md)、合成接口证据及 R2 任务 | 设计一致；关键接口可行性和产品适配缺口有证据 |
| R2 | 已交付：[本地 admin、默认空间、员工开户和成员 API](plans/WORKSPACE_CENTRIC_R2.md)，[验收及截图](plans/WORKSPACE_CENTRIC_R2_ACCEPTANCE.md) | 幂等初始化；限定本地账号登录；无个人空间；首次 Viewer；移除不自动加回 |
| R3 | 已通过合成验收：[原生授权、模型/API、旧业务退役、四工具 MCP](plans/WORKSPACE_CENTRIC_R3_ACCEPTANCE.md) | 原生四角色及归属/共享语义；检索前鉴权；跨空间、会话与引用隔离 |
| R4 | 已通过合成验收：[原生 KB/Agent/成员界面、Viewer 对话、批量成员和移交](plans/WORKSPACE_CENTRIC_R4.md) | 四角色原生操作；模型无需员工密钥；切换空间；截图与前端兼容性 |
| R5 | 已通过合成验收：[员工网页小助手](plans/WORKSPACE_CENTRIC_R5_ACCEPTANCE.md) | 真实员工身份；渠道/员工撤销；独立网页 OAuth、历史和引用 |
| R6 | 待实施：首次安装、恢复、试点和发布 | 真实身份/模型/代理；备份恢复；质量与运行证据；不把代码完成当作上线 |

权威要求见[总体设计](OVERALL_DESIGN_ZH.md#18-交付路线图)，阶段步骤见[改版方案](plans/WORKSPACE_CENTRIC_REDESIGN_ZH.md)。已授权阶段内逐项验证并继续，进度汇报不作为重复确认点。

## 已取代与保留的历史事项

| 历史事项 | 本次处理 |
|---|---|
| 知识库 publication/catalog/subscription、组织公开及产品 grant 链 | R3 新组合已停止注册并返回 410；R4 bundle 已移除旧界面；历史代码/迁移保留 |
| Personal Notes；P1-22 Wiki estimate、P1-23 Wiki build lifecycle | 随笔记方向取消/被取代，不补勾为开发完成；原生 Wiki 另按对照表恢复 |
| P1-24 UI/exclusion cleanup | 仍适用的排除与截图要求并入 R4，旧笔记/订阅界面要求被取代 |
| P1-25 Release regression/packaging | 有效历史证据保留，新增版本的回归和打包归 R6 |
| Phase 5 模型、OAuth、备份、观测和迁移工具 | 保留实现；按新入口和目标环境重验适用项 |
| 原 Phase 6 自研 GraphRAG、Phase 7 PixelRAG、Phase 8 本体 | 后置，本次不启动；原生 Wiki/图谱不是这些自研阶段的同义词 |
| 旧 Agent 发布 AP 计划 | 员工网页部分由 R5 取代，删除旧发布/订阅前提；小程序等更广渠道不并入 |

原任务和当时测试结果保留于[历史索引](archive/README.md)及[旧总体设计](archive/design/overall-design-v0.7-zh.md)。退役需求不等于物理删除数据库表、源文档或本地开发数据。

## 后置扩展

- 部门批量加入：先复用 R4 邮箱/CSV 批量能力，再接可信企业目录。持续同步需另定转岗、离职、手工角色覆盖和冲突规则。
- [桌面端设计](plans/DESKTOP_LOCAL_DESIGN_ZH.md)和自研 PixelRAG/本体保留为候选，不与本次主线并行实施。
- [旧企业渠道设计](plans/AGENT_PUBLISHING_CHANNELS_DESIGN_ZH.md)仅作背景，执行范围以 R5 员工网页小助手为准。

## R5、R6 收尾后的新增功能

2026-09-22 登记：知识图谱展示、知识图谱人工修订、知识版本管理、知识自动清洗、Wiki 下载、定时知识处理、发布聊天界面部分按钮屏蔽。七项均在 **R5、R6 开发收尾后**再逐项细化并分步骤实现，不并入当前主线交付。

范围、依赖与待定细节集中记录于[后续功能清单 E-01 至 E-07](plans/POST_R6_FEATURE_BACKLOG_ZH.md)。登记顺序不代表优先级，也不表示已实现；后续启动时再确定实施顺序和验收标准。

## R4 图谱增量

已交付[可选 Neo4j/APOC 补充包及专项证据](plans/R4_KNOWLEDGE_GRAPH_ACCEPTANCE.md)：文档抽取、持久化、重解析/删除、受限问答与真实浏览器配置。通过快速问答 Agent 使用图谱；普通 Agent `query_knowledge_graph` 的原生 service 接口提案后置，不修改上游。该增量不启动原 Phase 6 自研 GraphRAG。

## 后续目标环境验收

- 真实企业 OAuth 浏览器往返、首次开户、停用和退出；本地 admin 登录与恢复。
- 实际模型连通、解析/OCR、引用质量、延迟与成本；原生 Wiki/图谱仅在依赖和专项验收齐备时开放。
- 目标物理 AMD64 的 ParadeDB/BM25 预检：本机 ARM 上模拟 AMD64 会异常退出，ARM64 对照正常；根因及物理 AMD64 结果未确定，不把向量问答通过替代混合检索验收。
- 目标服务器备份恢复、告警、证书/代理及实际镜像版本证据。
- R5 独立网页的 Cookie/CSP/Origin/SSE、会话和引用隔离。
- 按适用发布流程完成试点和推广。已有有效证据可复用，历史记录不能替新版本签字。

[旧部署手册](guides/FULL_DEPLOYMENT_RUNBOOK_ZH.md)描述 Phase 5 历史组合；[R4 AMD64 企业部署手册](guides/R4_AMD64_ENTERPRISE_DEPLOYMENT_ZH.md)现已提供离线包初始化、恢复和调测步骤，R6 继续负责目标环境验收与推广。
