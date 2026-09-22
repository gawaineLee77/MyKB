# R4 原生知识图谱：离线启用与使用

此扩展包适用于 `mindcreek-r4-20260916-118b8af3-amd64` 企业包及其网关补丁。新增 Neo4j Community 2025.10.1 与同版本发行镜像自带的 APOC Core，复用原生知识库、默认 Chat 模型和问答流水线。验收结果见[专项记录](../plans/R4_KNOWLEDGE_GRAPH_ACCEPTANCE.md)。

## 1. 能力与边界

- 配置关系类型和示例，抽取文档中的实体、关系并写入 Neo4j。
- 知识库问答和快速问答 Agent 使用图谱召回源文档片段，保留引用与原生空间权限；员工无需提供模型密钥。
- 原生 Wiki 页面链接图与本功能不同。知识库设置中的实体关系画布用于编辑/预览抽取示例，不是全库图数据库管理器。
- 固定上游 v0.8.0 的普通 Agent `query_knowledge_graph` 工具当前委托 `HybridSearch`，不能将它的普通搜索结果当作 Neo4j 图查询验收；本扩展使用原生问答流水线及快速问答 Agent 的实体召回路径。
- 模型需能遵循结构化 JSON 抽取要求。合成模型验证接口和入库/查询链路；实际企业文档抽取准确率、耗时及模型费用需用代表性样本评估。

## 2. 准备

沿用正在使用的实例目录与项目名。下面路径是示例，按原部署填写。现有 OAuth 配置、CA 和默认模型保持；新网关包含短 Client Secret 与调用链日志修复；无需 init 或重新创建账户。

补充包包含 `mindcreek-neo4j:2025.10.1-graph1`、`mindcreek-gateway:r4-20260916-graph1` 和三个图谱预览接口的路由配置。原八镜像完整包继续保留，UI/app 不变。已知原网关、短 Client Secret 或 identity-diag1 补丁可直接升级；自定义网关镜像拒绝覆盖，需先评估差异。Neo4j 不发布宿主端口，仅通过同一 Compose 私有服务网络访问 Bolt 7687。启动不联网下载插件：镜像中已经包含 APOC，做法依据 [Neo4j 官方插件部署说明](https://neo4j.com/docs/operations-manual/current/docker/plugins/)。

初始 Java 堆为 512 MiB、最大 1 GiB、页缓存 512 MiB；还需 JVM/系统开销。可在实例 override 中调整 Neo4j 内存配置，按实际图谱量监测主机内存和磁盘。增加的卷名为 `${MINDCREEK_PROJECT}_mindcreek_graph_data`，不复用 PostgreSQL 卷。

## 3. 配置并启用

```sh
sha256sum -c mindcreek-r4-20260916-graph1-amd64-addon.tar.gz.sha256
tar -xzf mindcreek-r4-20260916-graph1-amd64-addon.tar.gz
cd mindcreek-r4-20260916-graph1-amd64-addon

export MINDCREEK_STATE_DIR=/srv/mindcreek/r4-instance
export MINDCREEK_PROJECT=mindcreek-enterprise-r4
python3 graph.py enable --package /opt/mindcreek/mindcreek-r4-20260916-118b8af3-amd64
```

工具验证扩展包和基线、导入 AMD64 镜像、生成随机数据库密码，并备份/合并实例 `compose.override.json`。新凭证保存在 `graph/password` 及私有 override 中，文件权限为 0600；不要输出完整 Compose 配置到工单。重复执行沿用密码和同一个卷；自定义 Neo4j 服务、凭证、网关镜像或路由/能力挂载发生冲突时拒绝覆盖。中断后修正错误并重复同一命令恢复。

工具只准备配置，不操作正在运行的服务。随后在维护窗口进入原完整包目录：

```sh
cd /opt/mindcreek/mindcreek-r4-20260916-118b8af3-amd64
python3 bin/mindcreek validate
python3 bin/mindcreek compose up -d --pull never --no-build --wait --wait-timeout 600 neo4j
python3 bin/mindcreek compose up -d --no-deps --pull never --no-build --force-recreate --wait --wait-timeout 600 app gateway
python3 bin/mindcreek compose restart frontend
python3 bin/mindcreek compose ps
```

Neo4j 的健康检查会通过真实认证执行 `apoc.version()`；健康后再重建 app。网关能力开关、能力文件 `rag_graph`、图谱路由挂载和 app 的 `NEO4J_ENABLE` 由工具同步配置。单独修改 `enterprise.env` 中的某个开关不足以启用本扩展。

## 4. 在页面中实际使用

1. 用该空间 Owner/Admin 登录，先新建一个测试文档知识库。原生抽取预览接口要求 Owner/Admin；Contributor 保持原生自有资源权限，不能因此获得管理员预览权限。
2. 打开知识库右上角设置，选择“知识图谱”，开启“启用实体关系提取”。保存时原生配置同步启用图谱索引。保留现有向量/关键词策略；图谱是补充召回方式。
3. 填写关系类型（例如“依赖于”“负责”）：输入后点击下拉列表中的新选项以加入标签，再填写示例文本和可选抽取要求。点击提取实体关系，检查节点、关系，保存知识库。
4. 上传少量测试文档。等待文档处理及图谱后处理完成；失败时查看文档处理详情和 app 日志。图谱每个分块可能调用模型，先小批量评估。
5. 在对话中选择这个知识库，提问实体之间的关系，并打开引用核对来源。也可创建快速问答 Agent，固定关联该知识库，供有访问权限的员工使用。

已完成处理的文档不会因修改知识库开关自动补建图谱。先选一篇旧文档使用原生“重建/重新解析”，确认成功后再逐步处理其余文档；不要一次性重建全部生产知识库。

初始示例可用合成文本：“Astra 服务依赖 Helios 数据库，Boreal 团队负责 Astra 服务。”关系类型填写“依赖于”和“负责”；回答应有真实文档引用，不能仅凭模型生成的答案判断图谱是否生效。

## 5. 排查

| 现象 | 检查项 |
|---|---|
| `graph.dependencies_unavailable` | 图谱能力/网关开关是否同步、app 是否报告 Neo4j、是否重建 app/gateway |
| Neo4j 不健康 | 镜像版本、磁盘/内存、`neo4j` 日志；不要给已有数据库重新生成初始密码 |
| 找不到 `apoc.*` | 应使用扩展包自带镜像；官方基础镜像尚未把插件复制到 plugins |
| 预览返回 `feature.disabled` | 确认网关已升级至 graph1，且新路由挂载已生效；原完整包路由仍关闭此接口 |
| 页面没有图谱配置 | 确认是文档知识库、具有原生管理权限、app 已重建且数据库引擎显示 Neo4j |
| 抽取为空或失败 | 默认 Chat 模型的结构化输出能力、文档内容、关系类型与处理任务错误 |
| 旧文档没有图谱 | 对选定文档重新触发处理并等待图谱任务完成 |

使用 `python3 bin/mindcreek compose logs --since 15m --no-color neo4j app gateway` 在服务器查看错误；日志可能含模型/文档错误信息，分享前移除私密内容。

## 6. 暂停与备份

暂停图谱时，回到扩展包目录执行：

```sh
python3 graph.py disable --package /opt/mindcreek/mindcreek-r4-20260916-118b8af3-amd64
```

再进入原完整包目录，重建 app/gateway、重启 frontend，然后执行 `python3 bin/mindcreek compose stop neo4j`。disable 保留新版网关、数据库卷、密码和知识库配置，同时关闭图谱能力；不回滚既有登录修复。图谱单独作为唯一索引的知识库暂停后将无法正常召回；生产通常保留向量/关键词作为其他检索方式。

恢复时重新执行 enable 和第 3 节启动步骤，沿用同一 project、实例目录及数据库卷。不要运行 `down --volumes`。

图谱卷必须纳入备份，不能只备份 PostgreSQL。使用维护窗口暂停入口和 app 工作进程，停止 Neo4j 后进行 Neo4j 离线 dump 或冷备图谱卷，并同时保存该时间点的 PostgreSQL、文档文件、实例秘密和镜像锁。不同时间点混合恢复可能造成文档 ID/图节点不一致。离线 dump/load 操作依据 [Neo4j 官方说明](https://neo4j.com/docs/operations-manual/current/docker/dump-load/)。目标服务器的完整恢复演练仍属于 R6。
