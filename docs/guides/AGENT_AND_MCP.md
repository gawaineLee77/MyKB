# 智能体问答与 MCP 接入

下文原 Phase 4/5 六工具与订阅说明仅适用于历史运行组合。R3 新组合以本节为准。

## R3 原生权限与四工具

R3 仅注册 `list_knowledge_bases`、`search_knowledge`、`get_source_excerpt`、`ask_knowledge_agent`。工具发现要求认证；已删除工具返回标准 Unknown tool 错误。员工/admin 使用原始 bearer，受限机器 Key 使用原始 `X-API-Key` 和当前 `X-Tenant-ID`，按原生能力与 KB 范围执行。机器会话、限流和审计按 Key 指纹分开；撤销 Key 与移除员工是独立操作。

显式 KB 越权整体拒绝，默认只选当前可访问原生资源。只读允许必要会话/审计，不允许写知识、Agent 或成员。MCP 拒绝带写 Wiki、数据库/外部工具、技能或记忆的 Agent；安全的 quick-answer 和明确只读知识工具可用。问答固定托管默认模型，受限 chat Key 不必获得模型管理权限。

原生 UI 在 R4 恢复，R3 不要求旧 `/platform/mindcreek/ask` 完整可用。协议约定仍见下文；接口和限制见[R3 验收](../plans/WORKSPACE_CENTRIC_R3_ACCEPTANCE.md)。

## 历史运行组合

本文从旧 Phase 4 运维记录中提取仍在使用的操作说明；部署和模型配置以[完整手册](FULL_DEPLOYMENT_RUNBOOK_ZH.md)为准。

## Web 问答

打开 `/platform/mindcreek/ask`。默认知识范围包括自己的、显式分享的和有效订阅的 KB；组织公开但未订阅的 KB 需要显式选择。撤销分享、取消订阅或取消发布后，新请求与引用打开都重新鉴权。

使用 Quick Answer 前确认所选知识库/智能体已关联可用聊天模型。Smart Reasoning 需要上游对应智能体配置有效的 KnowledgeQA 与 Rerank 模型；托管默认模型可用不等于任意旧智能体的模型绑定已经修复。

只聊天、不检索时，创建自定义智能体、绑定可用聊天模型，并将知识库选择模式设为 `none`。这不会开放文档读取权限，也不会为纯聊天生成知识库引用。空知识范围并非所有检索接口都接受。

## MCP 地址与身份

- 入口：`POST https://<MindCreek域名>/mcp`，走同一个受保护的前端入口，不直接暴露 Gateway。
- 使用有效的 `Authorization: Bearer <MindCreek token>` 或管理员批准的 `X-API-Key`。不把企业 OAuth 客户端密钥当作 API Key，也不将共享 Key 放进门户或小程序前端。
- 所有工具只读，沿用当前用户、租户和知识库授权；公开/订阅状态变化仍生效。
- 本实现不提供 hosted stdio、匿名访问、写入工具或服务端订阅流。HTTP GET/DELETE 返回 405 并不代表 POST 工具调用不可用。

## 六个工具

| 工具 | 用途 |
|---|---|
| `list_knowledge_bases` | 列出调用方默认授权知识范围中的 KB |
| `search_knowledge` | 搜索获准知识，不生成回答 |
| `get_source_excerpt` | 按 `chunk_id` 获取限长来源片段，每次重新校验权限 |
| `ask_knowledge_agent` | 在允许的知识范围中调用问答能力 |
| `list_publications` | 查询可见的内部发布目录 |
| `list_subscriptions` | 列出当前调用方的有效订阅 |

先调用 `tools/list` 获取当前实例的参数 schema，不自行添加接口未声明的参数。检索工具的 `knowledge_base_ids` 表示显式范围；省略时使用默认范围。GraphRAG/PixelRAG 仍是未开放的产品配置，不能仅通过 MCP 参数启用。

## 当前实现的协议匹配要求

以下是当前[网关实现](../../services/gateway/internal/mcp/handler.go)的约定，不表示其他版本客户端都能不经适配直接使用：

```http
Content-Type: application/json
Accept: application/json, text/event-stream
MCP-Protocol-Version: 2026-07-28
Mcp-Method: tools/list
```

同时携带前述一种认证凭据。列出工具的请求体：

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/list",
  "params": {
    "_meta": {
      "io.modelcontextprotocol/protocolVersion": "2026-07-28",
      "io.modelcontextprotocol/clientCapabilities": {}
    }
  }
}
```

调用工具时，将请求头 `Mcp-Method` 与请求体 `method` 同时改为 `tools/call`；增加 `Mcp-Name: search_knowledge`，并在 `params` 中加入 `"name": "search_knowledge"` 和 `"arguments": {"query": "测试问题"}`，保留同一 `_meta`。请求头的工具名必须与 `params.name` 一致。

出现 `-32020 Protocol header or metadata mismatch` 时，逐项检查 Accept 的两个媒体类型、协议版本、Mcp-Method、Mcp-Name 和 `_meta`。也保留了批准的 2025 协议初始化兼容路径，但不要混用新旧元数据规则；以客户端协商和源码测试为准。

## 验证与后续边界

`make phase5-check` 运行静态/单元契约；`make phase4-gate-c` 是需要运行中测试环境的合成 MCP 验收，会准备测试数据，不是生产巡检命令。历史证据见[Gate C](../archive/phase4/PHASE4_GATE_C.md)。

面向公司门户、小程序发布某个智能体是[未来渠道设计](../plans/AGENT_PUBLISHING_CHANNELS_DESIGN_ZH.md)，不是现有只读 MCP 的自动附加能力。
