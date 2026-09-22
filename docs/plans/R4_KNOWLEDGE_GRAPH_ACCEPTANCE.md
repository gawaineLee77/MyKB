# R4 原生知识图谱专项验收

日期：2026-09-16。范围：现有 R4 企业包的可选 Neo4j/APOC 扩展。固定 WeKnora v0.8.0 / `1edcd54b43606d9079bb36650efe3f68707a79ea`，上游源码无修改。原 R2–R4 验收报告保留，当前增量证据独立记录；本记录不替代 R6 企业目标环境验收。

## 1. 交付内容

- `mindcreek-neo4j:2025.10.1-graph1`：内置发行镜像自带 APOC Core，运行时不下载插件；独立数据卷、随机密码、内部 Bolt、真实认证健康检查。
- `mindcreek-gateway:r4-20260916-graph1`：接通三个原生图谱预览接口，检查图谱依赖与有效 Chat 模型；保留短 Client Secret、登录链路日志和原有空间授权。
- 配置工具合并现有企业实例，同步 app/gateway/installer、能力和路由文件；不更改 OAuth、CA、模型、账户或其他卷。支持锁、原子写入、私有备份、重复启用、停用及恢复；自定义配置冲突明确拒绝。
- 原 UI/app 镜像不变，使用原生知识库设置、抽取预览和员工对话页面。部署与操作见[指南](../guides/R4_KNOWLEDGE_GRAPH_ZH.md)。

## 2. 独立验证证据

| 项目 | 证据与结果 |
|---|---|
| API / MCP / R2 回归 | [合成接口记录](evidence/graph-api-probe.json)：真实原生 app、PostgreSQL、队列和 Neo4j；合成 OAuth 与确定性模型 |
| 真实图谱链路 | 示例预览；两个空间分别抽取文档；Cypher 核对节点/关系；关闭向量、关键词和 Wiki 的图谱唯一索引仍返回知识引用，防止普通检索兜底造成误判 |
| 身份与隔离 | Viewer 配置写入/预览拒绝；混入其他空间 KB 整体拒绝且模型调用数不变；受限 API Key 仅访问授权 KB；快速问答 Agent 使用服务器配置范围 |
| 生命周期 | Neo4j 重启保留节点；文档重新解析不重复节点；原生删除清理目标文档图谱，另一空间节点保留 |
| 部署工具 | [应用记录](evidence/graph-apply.json)：真实打包工具及镜像加载、并发/重复启用、停用恢复、凭证/卷/TLS/企业配置保留、Compose 合并有效；未启动企业实例 |
| 前端 | [浏览器记录](evidence/graph-browser.json)：真实 Vue 与原生 API，开启/自定义关系类型/预览/保存/刷新；员工从界面选择图谱 Agent，流式来源校验 |
| 工程检查 | [检查记录](evidence/graph-checks.json)：Go 1.26 无缓存全量测试、授权/MCP/身份/安装/服务竞态检查；Node 24 的 594 项前端测试、类型检查和构建；Python 3.12 的 7 项配置测试；路由、文档与上游边界检查 |
| 发布指纹 | [增量指纹](evidence/graph-release.json)：源文件、镜像、网关二进制、路由及证据摘要；`make graph-check` 核对本轮证据，旧 `r4-check` 仍针对历史全量版本 |

前后截图已人工检查；全部内容与账户均为合成数据：

- [开启前](../assets/graph/before-extraction-enabled.png)
- [抽取实体成功](../assets/graph/after-extraction-preview.png)
- [保存并刷新后](../assets/graph/after-saved-graph-settings.png)
- [员工图谱问答](../assets/graph/after-employee-graph-answer.png)

## 3. 权限与原生能力边界

三个 `POST /api/v1/initialization/extract/{fabri-tag,fabri-text,text-relation}` 保留上游 Owner/Admin 守卫和 Key 能力判断，不扩大 Contributor 权限。图谱关闭或原生服务未报告 Neo4j 时返回 `graph.dependencies_unavailable`。写操作继续由原生写接口授权，不以读取权限代替。

节点提取使用原生 `chat_pipeline/extract_entity.go`，图谱召回使用 `chat_pipeline/search_entity.go`。Neo4j 命中后映射回有权限的知识分块与引用；客户端不获得 Cypher、数据库密码或全库浏览接口。原生设置中的实体/关系编辑用于抽取示例，不是已入库图谱的全库可视化管理器。

**普通 Agent 工具限制：**固定上游 `internal/agent/tools/query_knowledge_graph.go` 当前调用 `knowledgeService.HybridSearch`。本次验证的是原生 RAG/快速问答流水线，不能把普通 Agent 此工具的结果声明为 Neo4j 查询。MCP 的 `ask_knowledge_agent` 可以使用已验证的路径；`search_knowledge` 仍沿用原生 HybridSearch。

最小后续上游提案：为普通 Agent 图谱工具提供复用原生实体召回的 service 接口，接收原生主体及受限 KB/文档范围，空范围不得退化为全空间；返回原生分块/引用。验证纯图谱、跨空间、共享撤销、受限 Key、图数据库不可用和普通检索差异。在固定 v0.8.0 上调查，尚未申请或应用补丁；需检索架构审查，上游公开接口支持后移除临时适配。当前可使用快速问答 Agent 完成图谱问答。

## 4. 未替代的企业验收

- 本机为 ARM64 上运行 AMD64 镜像；物理 AMD64 服务器、实际企业模型、真实文档抽取准确率、吞吐、延迟和费用仍需小样本验证。
- 基线包已有的 AMD64 模拟 BM25 异常未在此修复。图谱唯一索引用于证明图谱链路，不构成混合检索已验收；企业服务器仍运行原包数据库预检。
- 企业真实 OAuth、公司证书和账号未用于本轮测试，也未操作企业服务器。已有登录修复由新网关保留。
- 图谱卷重启持久化与停用恢复已验证；跨服务器全量备份/恢复演练、容量和监控纳入 R6。图谱、PostgreSQL 与文档文件需使用一致的恢复点。
- 不迁移或删除历史业务数据、旧迁移或已有开发卷；合成验证结束只删除本次随机项目的容器与临时卷。
