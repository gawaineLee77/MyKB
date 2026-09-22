# 社区帖子整理与 MindCreek 导入工具

本工具将接口获取的帖子整理为可导入文档，支持正文、S3 图片、主文档与附件。工具位于 [`tools/community-import/`](../../tools/community-import/README.md)，独立于 WeKnora 和运行中的服务，不需要重建 Docker 镜像，也不启用产品中关闭的上游 CLI/外部连接器。

R3 已改为原生 `/knowledge-bases` 建库、`/knowledge-bases/:id/knowledge/file` 上传和 `/knowledge/:id/reparse` 重解析；不再依赖 product profile、grant 或 ingestion 表。显式 `create-kb --name` 会保存建库意图，重复执行查询已创建 ID，未知结果先核对而不盲目重发。原生写权限由当前员工/admin bearer 或授权 ingest/manage_kbs Key 决定。新测试结果见[R3 验收](../plans/WORKSPACE_CENTRIC_R3_ACCEPTANCE.md)，本指南历史工程结果保留为当时记录。

## 1. 能力与边界

| 来源 | 处理方式 |
|---|---|
| `contentType=1` | 将 HTML 中的 `<img src="s3Uuid">` 下载并内嵌；保留标题、段落、代码、表格等静态结构，移除脚本、事件属性和外部嵌入，生成自包含 HTML |
| `contentType=2` | 使用 `contentFileUuid` 下载主文件，按 `contentFileExtension` 校验；导入主文档及一份记录标题、摘要和来源的说明文档 |
| `contentType=3` | 保留 Markdown，处理普通内联、引用式图片及 HTML `img`；图片内嵌后不依赖本机相对路径或论坛登录 |
| 附件 | 通过 `attachments[].s3uuid` 下载。支持的文档独立入库；PNG/JPEG/GIF/WebP 图片包装成 HTML 资料，原图仍本地保留 |

严格读取接口字段 **`aritcleTitle`**，不是 `articleTitle`。文章身份由 `source_id + articleCode` 确定；摘要不替代正文。

正文主文件支持当前文档 RAG 的 MD、TXT、PDF、DOC/DOCX、XLS/XLSX、PPT/PPTX、CSV、HTML/HTM、JSON、XML。文件头校验只是初步校验，不替代真正解析或恶意文件检测。不会执行文件中的脚本，也不会递归抓取附件内部的超链接。

下载得到的 UTF-8 Markdown/HTML 主文件和附件也会处理其中的 S3 图片，并在 `originals/` 保留未转换原件。Word/PDF 等二进制文档交由现有 Docreader 解析，不在导入工具中重写其内部结构。

帖子图片来源支持裸 S3 UUID 和合法的 PNG/JPEG/GIF/WebP Base64 data URI；不自动下载内容中的任意 HTTP URL、相对路径或 SVG。若接口改成其他资源引用形式，应先补充明确映射，不要放开任意 URL 下载。

Markdown 支持普通图片语法；围栏代码、行内代码和转义的图片示例保留原文，不触发下载。仅以缩进标识的代码示例请改用围栏代码块，避免与列表中的真实图片混淆。无法识别的活动图片语法会明确报错。

本工具不创建/发布知识库、不复制来源 ACL、不推断来源删除。请先按主题和统一访问权限创建受限的「文档 RAG」KB；文档上传、解析和模型调用可能消耗存储及模型额度。

## 2. 环境与初始配置

使用 Python 3.12 或更新版本，无第三方包依赖。支持 Linux/macOS，Windows 使用 WSL。以下命令从仓库根目录运行。

```sh
python3 tools/community-import/import_posts.py init
```

生成 `.local/community-import.json`，权限为 `0600`；已有文件不会覆盖。修改其中的接口占位符，**不要修改、提交真实凭据到仓库中的示例配置**。

| 配置项 | 必须填写的内容 |
|---|---|
| `source_id` | 稳定的来源系统名称；不要在同一数据目录中改变来源身份 |
| `articles.request` | 帖子接口 URL、GET/POST、查询或请求体；已有本地帖子 JSON 时可暂不填写 |
| `articles.items_path` | 文章数组的位置，例如 `data.items`、`data`，根数组用 `$` |
| `s3.request` | S3 下载接口 URL 和 UUID 参数名，值使用 `{s3uuid}` 占位符 |
| `mindcreek.base_url` | 当前服务的外部地址，不是内部 App 地址 |
| `mindcreek.knowledge_base_id` | 已创建的目标文档 RAG KB ID；不填名称，也不用 FAQ/个人笔记 KB |
| `headers_env` | HTTP 头到环境变量名的映射；环境变量的值是完整头值，例如完整 `Bearer …` |

默认读取 `COMMUNITY_AUTHORIZATION`、`COMMUNITY_S3_AUTHORIZATION`、`MINDCREEK_IMPORT_API_KEY`。通过受控 Secret 管理或当前会话注入，不把值写入配置、命令参数或 Git。若接口不需要认证，可删除对应的 `headers_env` 项；若使用 Cookie，也通过环境变量映射 `Cookie` 头。

在 Bash 中可以逐项隐藏输入（每次输入后回车），避免真实值写入命令历史：

```sh
read -r -s COMMUNITY_AUTHORIZATION
export COMMUNITY_AUTHORIZATION
read -r -s COMMUNITY_S3_AUTHORIZATION
export COMMUNITY_S3_AUTHORIZATION
read -r -s MINDCREEK_IMPORT_API_KEY
export MINDCREEK_IMPORT_API_KEY
```

## 3. 帖子 API 适配

默认示例是 GET 分页，参数中 `{page}`、`{page_size}` 会在运行时替换。POST 可配置 `json` 或 `form`，二者不能同时使用：

```json
{
  "url": "https://REPLACE_ARTICLE_API.example.invalid/list",
  "method": "POST",
  "headers_env": {"Authorization": "COMMUNITY_AUTHORIZATION"},
  "json": {"pageNo": "{page}", "pageSize": "{page_size}"}
}
```

支持四种 `articles.pagination.mode`：

- `none`：只请求一次；`items_path` 可指向文章数组或单篇文章对象。
- `page`：配置 `start_page`、`page_size`、`max_pages`。
- `offset`：请求中使用 `{offset}`、`{page_size}`；配置 `start_offset`。
- `cursor`：请求中使用 `{cursor}`，配置 `next_cursor_path`，例如 `data.nextCursor`；结束值应为 `null` 或空字符串。

page/offset 默认在不足一页时结束；如果接口提供布尔 `hasMore`，请设置 `has_more_path`，避免服务端限制每页数量导致提前结束。请用接口总量核对首次导出的数量。重复页面、循环游标和超出 `max_pages` 会明确失败，不把截断结果当作完整获取。

若列表只含文章标识，可增加 `articles.detail`：

```json
{
  "request": {
    "url": "https://REPLACE_ARTICLE_API.example.invalid/detail",
    "method": "GET",
    "headers_env": {"Authorization": "COMMUNITY_AUTHORIZATION"},
    "query": {"articleCode": "{articleCode}"}
  },
  "item_path": "data"
}
```

上面的对象是 `articles.detail` 的值。详情返回的 `articleCode` 必须与列表项一致。

## 4. S3 下载适配与企业证书

默认 `s3.response_mode=binary`，接口直接返回文件字节。UUID 可以放在 URL、query、JSON 或表单中，例如：

```json
{
  "url": "https://REPLACE_S3_API.example.invalid/download",
  "method": "POST",
  "headers_env": {"Authorization": "COMMUNITY_S3_AUTHORIZATION"},
  "json": {"s3uuid": "{s3uuid}"}
}
```

如果接口返回下载 URL，则设置 `response_mode=json_url`、`url_path`，并在 `download_origins` 中列出允许的精确源，例如 `https://files.company.example`。下载 URL 请求**不会携带 S3 API 的认证头**。如果下载 URL 本身还需要额外认证，应增加专门适配，不能随意转发原凭据。

HTTP 重定向默认拒绝，避免认证信息随跳转泄露。默认必须 HTTPS；企业 CA 配置在 `http.ca_file`，或使用 Python/OpenSSL 支持的 `SSL_CERT_FILE`。工具沿用 `HTTPS_PROXY`、`HTTP_PROXY`、`NO_PROXY`；它运行在哪台机器，就需要在那台机器配置信任链，与 Gateway/App 容器中的 CA 是两件事。没有关闭 TLS 校验的选项；`allow_insecure_http` 只用于明确选择的 HTTP 测试/隔离环境，不用于解决 HTTPS 证书错误。

## 5. 推荐执行步骤

### 先做完全离线的整理验证

```sh
python3 tools/community-import/import_posts.py --output .local/community-import-demo prepare \
  --input tools/community-import/examples/offline-article.json
```

此样例仅含合成正文与内嵌像素图片，不访问帖子、S3 或 MindCreek。单独的数据目录避免将演示资料混入正式导入。

### 已经获取了所有帖子

支持单篇 JSON 对象、JSON 数组或每行一篇的 JSONL。若保存的是接口外层包装，请先取出文章数组；本地输入不会自动猜测 `data.items`。

大批量资料优先用 JSONL，避免一次将整个 JSON 数组读入内存。`lastUpdateDate` 可保留日期字符串或数值时间戳；工具按字段变化判断版本，不擅自推断时区或排序。

```sh
python3 tools/community-import/import_posts.py prepare --input .local/my-articles.json
python3 tools/community-import/import_posts.py status
```

这里仍会调用 S3 下载正文引用的图片、主文件及附件，但不调用帖子接口，也不上传到 MindCreek。

整理成功但尚未上传时，`status` 的 `uploads.not_uploaded` 大于零，退出码为 `2`，这是预期结果。`uploads` 同时列出 `completed`、`pending`、`failed` 和 `article_failures`，仅统计当前配置的目标服务/KB 与当前资料。离线状态依据最近一次保存的结果；重新核对远端使用 `check`。

### 从 API 获取并整理

```sh
python3 tools/community-import/import_posts.py fetch
python3 tools/community-import/import_posts.py prepare
```

`fetch` 原子写入工作目录的 `articles.jsonl`；分页失败保留上一份完整快照。`prepare` 可重复执行，未变化的文章会校验已有文件后跳过。并发操作同一工作目录会被锁阻止；进程退出后锁自动释放。

### 检查样本后上传

先在 MindCreek 确认 Chat/Embedding/Rerank 可用；需要图片检索时启用可用的 VLM 和 KB 图片处理，见[VLM 指南](PHASE5_OPERATIONS.md#scanned-and-image-only-pdfs)。

```sh
python3 tools/community-import/import_posts.py upload --wait-seconds 600
python3 tools/community-import/import_posts.py check --wait-seconds 600
```

`upload` 只调用产品 `/api/v1/knowledge-bases/{id}/ingestions`，不直连数据库、向量库或绕过网关。`check` 不上传新文件、不自动重解析，仅核对现有文档身份、权限和解析状态；`status` 完全不联网。

确认配置正确后可执行完整流程：

```sh
python3 tools/community-import/import_posts.py run --wait-seconds 600
# 使用已有帖子列表，跳过帖子 API：
python3 tools/community-import/import_posts.py run --input .local/my-articles.json --wait-seconds 600
```

退出码：`0` 当前命令成功；`1` 有错误或文章/解析失败；`2` 上传、检查或离线状态仍有未完成或尚未上传的文档；`130` 用户中断。解析完成不等于 OCR/检索质量已经验收，请分别测试仅存在于正文、图片、附件中的问题及其引用。

## 6. 目录、版本与去重

```text
.local/community-import/<来源名及哈希>/
  articles.jsonl
  state.json
  articles/<文章标识及哈希>/versions/<版本号>/
    source.json
    manifest.json
    body/
    images/
    attachments/
    originals/
```

`state.json` 指向每篇文章的当前可用版本，并记录按目标服务/KB 隔离的上传台账。`manifest.json` 记录来源、时间、资源、哈希、上传文件名和关联关系。文件权限为 `0600`，目录为 `0700`。这些内容包含内部资料，必须保留在忽略的本地目录或受保护的数据盘，不能提交 Git。

每篇帖子独立归档；同一篇内重复 UUID 复用下载结果。跨帖本地文件可重复保存，但同一工作目录、同一目标 KB 中相同哈希及后缀的入库资料会复用上传记录，不跨 KB 去重。帖子说明保留独立来源信息；资源关联只是导入管理关系，不代表 RAG 自动展开所有附件。

入库文件名包含可读前缀、安装标记和内容哈希，以便中断后核对。上传必须使用同一工作目录继续，不要丢弃 `state.json` 后反复重跑。工具不自动认领此前手工上传的相同文件；遇到上游重复文件拒绝时，应先核对既有资料，不能为消除报错直接删库。

## 7. 重试、更新与旧版本处理

- 改正 API/下载配置后重跑 `prepare` 或 `run`。一个帖子失败不破坏其上一份成功版本，其他帖子仍可整理。
- 同批出现相同文章 ID 的不同内容时，报告 `article.duplicate_id_conflict`，保留批次开始前的可用版本；先在输入中确定唯一版本再重跑。
- 更新时间、正文或资源清单变化会触发重建。若源系统在不修改这些字段的情况下替换了同 UUID 文件，用 `prepare --refresh` 强制重新下载。
- 修改图片限额或附件处理策略后，使用 `prepare --refresh` 重新整理已有文章，使新策略应用到缓存资料。
- 文件准备成功不等于上传成功；上传接收成功不等于解析完成。多个文件的远端上传不是原子事务，部分成功的记录会被保留以便续传。
- 已有远端文档解析失败时，先修复解析器/模型，再运行 `upload --retry-failed --wait-seconds 600`。这可能重新消耗模型额度。
- 上传响应中断时先通过安装标记、文件名和大小核对远端，不盲目重发 POST。无法确认时返回 `mindcreek.upload_unresolved`；检查服务器是否接收该文件。重跑可继续核对，但不会擅自重传未知结果的写请求。
- 普通流程不删除本地历史或远端旧文档，因此更新后可能暂时同时检索到新旧版本。请先在受限 KB 中验收新版本，再决定是否清理旧版。

旧版清理必须明确执行：

```sh
# 只读预览，列出候选文档 ID：
python3 tools/community-import/import_posts.py prune
# 确认预览与备份后，显式请求删除：
python3 tools/community-import/import_posts.py prune --apply
```

仅清理本工具确认创建、当前已不再引用的远端文档；全部当前替代资料必须完成解析，且没有失败/中断的准备任务。删除前重新核对 KB、ID、文件名和大小。共享附件仍被任何当前文章引用时不删除；响应中断后核对到但无法确认创建归属的文档不自动清理。删除是服务器异步请求，本地历史文件仍保留，可用于人工恢复。

本工具不根据“文章未出现在本次列表”自动删除内容，也不自动同步论坛权限撤销。若用于持续生产同步，需要额外接入权威删除/ACL 事件；当前版本适用于已获准资料的受控导入与更新。

## 8. 排错与验证

错误输出仅包含阶段、固定错误码和 HTTP 状态，不打印正文、响应体、下载签名、Cookie 或 Token。详细的文档解析错误在 MindCreek 中查看。

| 错误/情况 | 处理 |
|---|---|
| `config.endpoint_placeholder` | 对应接口仍是占位符；只整理本地自包含示例时无需填接口 |
| `config.section_invalid` / `config.request_invalid` / `config.selector_invalid` | 核对 JSON 层级：配置段和请求头映射应为对象，URL、方法和字段路径应为字符串 |
| `response.selector_missing` / `source.items_not_array` | 修正 `items_path`、详情路径或分页返回结构 |
| `http.status_error`，状态 401/403 | 检查该接口的环境变量认证头和权限，不复用不相关系统的 Token |
| `http.tls_verification_failed` / `http.tls_failed` | 检查工具所在机器的企业 CA 与证书链，不关闭 TLS 校验 |
| `http.timeout` / `http.dns_failed` / `http.connection_refused` / `http.transport_failed` | 检查代理、DNS、目标端口、网络和超时 |
| `s3.uuid_invalid` | 图片源不是裸 UUID/支持的 data URI，需要明确映射 |
| `document.signature_mismatch` | 文件字节与后缀不匹配，可能下载到了错误页；不要只改后缀 |
| `document.extension_unsupported` | 默认停止该帖子；转换附件，或设 `unsupported_attachments=archive`，只归档并报告未索引警告 |
| `storage.artifact_changed` | 已整理文件被修改；重新 `prepare --refresh`，不要直接篡改哈希 |
| 上传仍失败/413 | 核对目标服务实际上传限制；Base64 图片增大会计入最终文件大小 |

开发验证：`make community-import-test`。测试只启动本机合成 API，不证明真实公司接口或实际模型效果；投入使用前先用获准的少量资料完成接口和问答验收。

## 9. 工程交付验证

2026-09-11 完成工具工程收尾：获取、整理、上传、检查、离线状态、续传及显式旧版清理命令均已实现；公司帖子 API 和 S3 下载接口按要求保留占位配置。

- Python 3.12 与 3.14：各 41 项合成测试通过，包括三种帖子类型、图片和附件、失败恢复、目标 KB 隔离、鉴权拒绝、上传结果核对及旧版清理边界。
- 独立命令进程依次执行 `fetch → prepare → status → check → upload → check → status → run → prune`；三篇合成帖子产生五份去重后的入库资料，重跑没有新增重复上传，只读检查和清理预览没有写入或删除资料。
- `make phase1-gateway-test` 通过；导入 URL、分页、文件字段及返回结构与当前产品接口一致。
- 无 UI 变更，无需重建服务镜像。真实公司 API、真实解析/OCR 和问答引用质量留到填写配置后的样本联调，不计入以上合成验收。

默认测试使用 `python3`；可指定解释器：`make community-import-test COMMUNITY_IMPORT_PYTHON=python3.12`。仓库 CI 已配置 Python 3.12/3.14 测试矩阵，远程 CI 结果须以实际运行记录为准。
