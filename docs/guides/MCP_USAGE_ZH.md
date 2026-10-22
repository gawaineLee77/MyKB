# MindCreek MCP 能力与使用说明

更新：2026-10-20。适用已完成升级的 `current-20261019-d3fafdaf` 企业版本；R4 和 1009 旧版本不包含新增的两个 Wiki 工具。以服务器 `tools/list` 的实际结果为准。

## 1. 能力清单

| 工具 | 用途 | 参数 | API Key 能力 |
|---|---|---|---|
| `list_knowledge_bases` | 列出当前凭证可访问的知识库及 ID | `{}` | `retrieve` |
| `search_knowledge` | 检索文档片段，供外部智能体组织回答 | 必填 `query`；可选 `knowledge_base_ids`、`knowledge_ids` | `retrieve` |
| `get_source_excerpt` | 按分块 ID 读取来源原文片段 | 必填 `chunk_id`；可选 `max_chars` | `retrieve` |
| `ask_knowledge_agent` | 由 MindCreek 生成回答并返回来源引用 | 必填 `query`；可选 `knowledge_base_ids`、`agent_id`、`session_id` | `retrieve` + `chat` |
| `wiki_search` | 对已生成的 Wiki 做字面文本搜索 | 必填 `query`；可选 `knowledge_base_ids`、`limit` | `retrieve` |
| `wiki_read_page` | 分段读取指定 Wiki 页面的正文 | 必填 `knowledge_base_id`、`slug`；可选 `offset`、`max_chars` | `retrieve` |

六个工具均不修改知识内容。问答仍会创建/更新会话和审计记录，重试可能重复生成回答。

MCP 当前不提供文档上传、修改、删除、原文件下载、知识库配置、成员管理、Wiki 生成/重建/编辑、外观或智能体图标管理，也没有独立的图谱导出工具。部分内容写入可使用另行授权的 REST API，不能通过上述 MCP 工具完成。

## 2. 连接与授权

| 配置 | 内容 |
|---|---|
| 服务地址 | `https://你的服务域名/mcp`，经过产品入口 |
| 传输 | HTTP POST，JSON-RPC；选择客户端支持的 HTTP / Streamable HTTP 连接方式 |
| 机器认证 | `X-API-Key: <API_KEY>` |
| 目标空间 | `X-Tenant-ID: <十进制空间ID>`，必须与凭证所属/获准空间匹配 |
| 人类会话认证 | 可用 `Authorization: Bearer <MindCreek会话令牌>` 替代 API Key |
| 内容类型 | `Content-Type: application/json` |
| 接受类型 | `Accept: application/json, text/event-stream` |

每次请求选一种认证方式。外部长期集成使用独立 API Key；企业 OAuth Client Secret 不能作为 API Key，企业身份平台的令牌也不能直接替代 MindCreek 会话令牌。

在目标空间“设置 → API 集成”创建 Key：普通搜索、来源读取和 Wiki 仅需要 `retrieve`；需要生成回答时再加 `chat`。Owner 管理全部普通 Key；已获空间管理授权的 Admin 管理自己的 Key。

必须明确保存 Key 的产品知识库范围。Admin 指定具体知识库；仅 Owner 可显式授予全部知识库。旧 Key 未补齐产品范围映射时不能访问知识。请求参数只能收窄范围，不能通过填写私有库 ID 扩权；公开知识库也不会自动绕过机器 Key 的范围限制。

人类会话遵循当前 KB 可见性：创建者、Owner/Admin、公开库的空间成员及被指定成员按各自权限访问。可见、检索和读取文本不自动授予原文件下载权限。已有片段或 Wiki 引用仍会重新鉴权。

## 3. 命令行快速开始

下载本说明配套的 [mcp.sh](../examples/user-manual/mcp.sh)，在保存它的目录执行。需要 Bash、curl、jq。此脚本已补齐六工具白名单；旧离线包内的示例若仍写着 four tools，请单独替换示例脚本，无需改服务镜像。

```bash
export MC_ORIGIN='https://你的服务域名'
export MC_TENANT_ID='替换为目标空间数字ID'
export MC_AUTH_KIND='key'
read -r -s -p '输入 API Key: ' MC_API_KEY
printf '\n'
export MC_API_KEY

# 发现工具，并核对真实 inputSchema
bash mcp.sh tools/list

# 列出获准知识库，从结果 items 取得 ID
bash mcp.sh tools/call list_knowledge_bases '{}'
export MC_KB_ID='替换为知识库ID'
```

`MC_ORIGIN` 不带尾部斜杠。空间 ID 可从当前空间浏览器请求的 `X-Tenant-ID` 确认。企业 CA 未被客户端信任时，将 `CURL_CA_BUNDLE` 指向企业 CA 的 PEM 文件；不要使用服务器私钥。

### 文档检索与来源片段

```bash
bash mcp.sh tools/call search_knowledge \
  "$(jq -n --arg kb "$MC_KB_ID" \
    '{query:"设备维护周期",knowledge_base_ids:[$kb]}')" \
  > mcp-search.json

jq '.result.structuredContent.results' mcp-search.json

# 有检索结果时，使用其中的分块 ID
export MC_CHUNK_ID="$(jq -er '.result.structuredContent.results[0].id' mcp-search.json)"
bash mcp.sh tools/call get_source_excerpt \
  "$(jq -n --arg id "$MC_CHUNK_ID" '{chunk_id:$id,max_chars:4000}')"
```

`search_knowledge` 的 `query` 最多 4000 字符，最多返回 20 条片段，每条最多 4000 字符；没有 `limit` 参数。`knowledge_ids` 是可选的文档 ID 数组，最多 64 个。`get_source_excerpt.max_chars` 默认 4000，范围 1–12000。

### 由 MindCreek 生成回答

```bash
bash mcp.sh tools/call ask_knowledge_agent \
  "$(jq -n --arg kb "$MC_KB_ID" \
    '{query:"说明设备维护周期，并引用依据。",knowledge_base_ids:[$kb]}')" \
  > mcp-answer.json

jq '.result.structuredContent | {session_id,answer,references}' mcp-answer.json
```

不传 `agent_id` 使用服务托管的默认问答模型；传入时使用有权调用且符合只读约束的已有智能体，知识范围须与其配置兼容。`query` 最多 8000 字符。

继续对话时带回 `session_id`，保持同一凭证、空间和兼容知识范围。切换范围建议新建会话。不能复用其他员工、其他 Key 或 R5 渠道会话。返回完整 JSON，不提供逐字回答流。

空间记忆必须关闭；带知识写工具、任意外部 MCP、Skills、Web 搜索、沙箱或不兼容记忆配置的智能体不能用于此路径。遇到升级前的旧 Agent，需核对并重新保存兼容配置。

### Wiki 搜索与正文读取

```bash
bash mcp.sh tools/call wiki_search \
  "$(jq -n --arg kb "$MC_KB_ID" \
    '{query:"设备维护",knowledge_base_ids:[$kb],limit:10}')" \
  > mcp-wiki-search.json

jq '.result.structuredContent.results' mcp-wiki-search.json

# 有结果时，使用结果中的真实 slug，不用页面标题代替
export MC_WIKI_SLUG="$(jq -er '.result.structuredContent.results[0].slug' mcp-wiki-search.json)"
bash mcp.sh tools/call wiki_read_page \
  "$(jq -n --arg kb "$MC_KB_ID" --arg slug "$MC_WIKI_SLUG" \
    '{knowledge_base_id:$kb,slug:$slug,offset:0,max_chars:8000}')"
```

- Wiki 搜索是字面文本匹配，不是语义问答、正则搜索或 Wiki 生成。目标库应已启用 Wiki 并有生成的页面。
- `limit` 默认 10，最大 20；`truncated: true` 表示可能还有结果，可细化搜索词或范围。
- 省略 Wiki 搜索的知识库范围时使用当前凭证默认获准范围；显式传 `[]` 或 `null` 会拒绝。固定库集成建议始终传具体 ID。
- 正文 `max_chars` 默认 8000，最大 20000；存在 `next_offset` 时，以该值作为下一次 `offset`。偏移按 Unicode 字符计数。
- 上游单次页面响应限制为 1 MiB；超过时会报错。正文资源链接会进行权限检查和脱敏，不产生原文件下载授权。

## 4. 接入智能体客户端

在支持自定义请求头的 HTTP MCP 客户端中配置服务 URL、`X-API-Key` 和 `X-Tenant-ID`，让客户端执行协议协商、工具发现，再用一个已授权知识库验证检索。

当前网关兼容 `2025-03-26`、`2025-06-18`、`2025-11-25` 的 `initialize` 协商；配套脚本使用网关的 `2026-07-28` 接口约定，需要匹配 `Mcp-Method`、`Mcp-Name` 和请求 `_meta`。不要在不匹配的客户端上强制填写该版本。详细原始 HTTP 请求见[用户手册 9.7 节](MINDCREEK_USER_MANUAL_ZH.md#97-理解-http-请求)。

当前不提供 stdio 服务、GET SSE 订阅流、MCP resources 或 prompts 接口。直接用浏览器打开 `/mcp` 会发 GET 并得到 405。若客户端只支持旧式 SSE 地址或不能传必要请求头，不能仅填 URL 就保证可用；先用配套脚本验证服务，再核对该客户端的 HTTP 连接能力。各种客户端配置文件字段不同，本说明不提供未经验证的通用客户端 JSON。

外部智能体自行组织答案时，使用“列库 → search_knowledge → get_source_excerpt”；需要现成知识问答时使用 `ask_knowledge_agent`；读取已生成 Wiki 时使用“wiki_search → wiki_read_page”。

## 5. 常见问题

| 表现 | 核对事项 |
|---|---|
| 发现的只有 4 个工具 | 确认新网关已启动，客户端重新发现；R4/1009 旧版本没有 Wiki 工具 |
| 脚本提示 four tools | 使用本说明链接的更新示例；这是客户端脚本白名单，不是服务器工具缺失 |
| HTTP 401 | Key/会话令牌是否有效、是否撤销，认证头是否传到产品入口 |
| HTTP 403 / workspace_denied | 空间 ID 与凭证是否匹配 |
| 工具返回 Access denied | Key 能力、产品知识库范围、KB ACL、Agent/会话范围、空间记忆设置 |
| Wiki 无结果 | 是否启用并生成 Wiki，搜索词是否字面命中；查看 `searched_knowledge_base_ids` |
| HTTP 405 | 使用了 GET/DELETE；实际工具请求应为 POST |
| Protocol header or metadata mismatch | 协议版本、Accept、方法头、工具名和元数据是否一致 |
| HTTP 429 | 按 `Retry-After` 等待再试 |
| Tool temporarily unavailable | 查看网关请求关联 ID 对应日志，核对上游服务、托管模型、索引及页面大小 |

不能只看 HTTP 200 判断成功；还要检查 JSON-RPC 的 `error` 及工具结果。正常内容位于 `result.structuredContent`。配套脚本会将 HTTP 或 JSON-RPC 失败作为非零退出码，不会自动重试问答。

本说明依据[工具与协议实现](../../services/gateway/internal/mcp/handler.go)、[知识工具实现](../../services/gateway/internal/mcp/native.go)和[Wiki 工具实现](../../services/gateway/internal/mcp/wiki.go)整理；未连接企业服务器，也不据此宣称企业升级已完成。
