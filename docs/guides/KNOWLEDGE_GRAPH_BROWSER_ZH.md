# 知识图谱只读浏览（E-01）

本功能展示 Neo4j 中由文档实际抽取的实体关系，与 Wiki 页面链接图分开。当前源码已实现；已有 R4 图谱补充包不包含本页面和读取接口，需要升级网关与前端。实施和验证见[验收记录](../plans/KNOWLEDGE_GRAPH_BROWSER_ACCEPTANCE.md)。

## 使用

1. 切换到知识库所属空间，打开文档知识库详情，点击标题旁的“知识图谱”。
2. 输入实体名称进行包含匹配搜索；关系类型支持输入精确名称，候选项来自已加载图谱。空条件显示初始有限范围。
3. 点击实体查看提取属性，点击“展开相邻实体”加载一跳邻居；点击连线查看关系方向和类型。不同文档中的同名实体保持独立。
4. 点击“查看来源文档”打开现有文档预览；预览仍执行原生权限校验。
5. 滚轮或按钮缩放，拖动画布平移，拖动节点调整当前布局。刷新或复位不会修改数据库。

只对当前空间的 Owner、Admin、Contributor 显示入口。后端同时要求有效人类会话、对应空间成员角色和原生知识库读取权限；平台管理员身份不提供额外内容权限。Viewer、非成员和 API Key 均不能访问浏览接口。Contributor 的创建、编辑和共享权限没有变化。跨空间共享的库不在此浏览器范围内，需切到所属空间并具有规定成员角色。

知识库未启用提取、尚无实体、筛选无结果及数据库不可用分别显示提示。开启提取不会给旧文档自动补图，需按原生流程处理或重解析指定文档。

## 服务器配置

先按[原生图谱指南](R4_KNOWLEDGE_GRAPH_ZH.md)启用 Neo4j 和抽取。浏览器使用同一个私有 Bolt 服务，无需发布 Neo4j 端口。仅给网关增加以下变量，前端不接触数据库账号：

```dotenv
MINDCREEK_GRAPH_VIEW_URI=bolt://neo4j:7687
MINDCREEK_GRAPH_VIEW_USERNAME=neo4j
MINDCREEK_GRAPH_VIEW_PASSWORD=<已安装图数据库的实际密码>
MINDCREEK_GRAPH_VIEW_DATABASE=neo4j
```

使用同一个数据库、沿用已有密码，不重设 `NEO4J_AUTH`。企业版本如已配置独立数据库只读账号，可使用该账号。网关自身只执行固定只读查询，不提供任意 Cypher 或写接口；`AccessModeRead` 不是数据库账号的权限替代。

本仓库新的 `deploy/enterprise/compose.json` 已把上述变量从私有 `enterprise.env` 传给 gateway。安装于旧离线包的实例，需要将同名变量加入实例 `compose.override.json` 的 `services.gateway.environment`，并确保使用包含本功能的网关、前端镜像。保持 `MINDCREEK_GRAPH_ENABLED=true`、图谱能力文件和 app 的 `NEO4J_ENABLE` 配置一致。包含密码的实例配置维持 0600，不提交仓库。

在维护窗口使用原部署包装命令重新创建 gateway 和 frontend；无需数据库迁移、重新初始化账户或重建已有图谱。此次源码交付没有重建 AMD64 离线镜像包，也没有操作企业服务器。

## 接口和限制

`GET /api/v1/mindcreek/graph/knowledge-bases/{kb_id}`

携带原始 `Authorization: Bearer ...` 和当前 `X-Tenant-ID`。可选参数：

| 参数 | 含义 |
|---|---|
| `q` | 实体名称包含匹配，最多 100 字符 |
| `relation` | 关系类型精确匹配 |
| `node_id` | 展开此节点一跳邻居，节点仍必须属于 URL 指定知识库 |
| `limit` | 默认 80，范围 1–160；展开结果另保留中心节点 |

响应为 `{success:true,data:{nodes,edges,truncated,state}}`。实体含稳定于当前图数据库对象的 `id`、`name`、`knowledge_id`、`attributes`；关系含 `id`、`source`、`target`、`type`。节点重新抽取后 ID 可能变化，不作为业务永久标识。单次最多返回 320 条关系，页面累计最多 240 个实体、480 条关系，出现截断提示后用搜索缩小范围。搜索或筛选会重新加载，展开会合并当前视图。

每次加载、搜索和展开重新核验权限；返回前过滤已删除、已移库或失去读取权限的来源文档。不承诺撤销瞬间抹去用户已看到的内容。图查询超时 5 秒，整体请求最多 15 秒；大图可能超时并提示重试，不执行全库无界导出。

常见错误：`graph.role_denied`（角色不允许）、`workspace.denied`（空间不符）、`graph.dependencies_unavailable`（未启用或连接未配置）、`graph.unavailable`（数据库不可用）、`graph.source_unavailable`（来源核验暂时失败）。

节点、关系的人工修改归 E-02。本功能没有编辑、合并、删除图实体的能力，不改变原生抽取任务。
