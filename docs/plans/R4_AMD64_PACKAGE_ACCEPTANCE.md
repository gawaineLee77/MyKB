# R4 AMD64 企业调测包：构建与验证记录

日期：2026-09-16。版本：`r4-20260916-118b8af3`。

## 结论

八个 Linux AMD64 镜像及独立部署工具已构建。**该包用于企业内网调测，关键词/混合检索尚未通过 AMD64 验收，不代表生产放行。** 目标物理 AMD64 必须先运行包内 `bin/check-database`，通过后才能导入企业数据。

完整步骤见[企业部署手册](../guides/R4_AMD64_ENTERPRISE_DEPLOYMENT_ZH.md)。输出目录为仓库的 `images/archives/mindcreek-r4-20260916-118b8af3-amd64/`，同目录提供 `.tar.gz` 和 `.tar.gz.sha256`。

## 已验证

- 8 个镜像全部为 `linux/amd64`，已验证 Docker save 归档、重新 load、平台配置摘要和镜像锁；[归档证据](evidence/enterprise-amd64/archive-check.json)。
- 19 项实际运行检查：受控 admin 初始化、重复执行不重复建空间、六个核心服务健康、Nginx/TLS、管理员登录、自签 CA 下合成 OAuth/Viewer 开户、默认模型、显式向量库、真实 DocReader Markdown/文本 PDF、FAQ、Viewer 写入/下载限制、MCP 四工具与向量问答引用、退役接口、关闭密码注册、网关重启和停写备份。
- 2 项独立恢复检查：从上述运行产生的备份恢复到新库/新卷；2 个知识库、2 个账户、3 条知识记录、安装状态 ready；恢复 6 个文件，Redis 恢复后 healthy。[恢复证据](evidence/enterprise-amd64/restore-check.json)。
- Python 3.12 AMD64 容器内 10 项安装工具测试通过：密码边界、秘密权限、不覆盖初始化、字面环境变量、Shell 隔离、失败关闭注册、平台镜像锁、校验路径、一次性写入权限和私有 OIDC 端点。
- 设计/链接/格式检查、当前 R4 证据校验和 `git diff --check` 通过；固定上游源码保持干净。产品业务源文件和前端静态包未改变，原 R4 Node 24 构建指纹继续有效。

[汇总证据](evidence/enterprise-amd64/acceptance.json)明确记录来自同一运行配置的主流程与恢复补充验证，没有把早期失败覆盖成从未发生。

## 本次部署修正

1. 发布镜像补齐公开 CA；实例可合并企业 CA，保持 HTTPS 验证。
2. 前端镜像使用完整 Nginx 入口模板及必需代理 include，避免旧模板注释匹配破坏配置。
3. 管理员密码在建号前验证原生最大 32 位等边界。
4. Compose 5 的 `run -v ...:rw` 不会覆盖原只读属性，改为专用一次性 installer；正常 gateway 仍只读。
5. 过滤宿主 Shell 对产品环境变量的隐式影响；内部 OIDC Discovery/JWKS 固定到私有 gateway。
6. 默认 warn 日志；原生 info 日志会包含模型请求正文，不能把关闭额外 LLM 日志文件误认为隐藏全部请求正文。
7. 首轮恢复遇到 ParadeDB 预建 schema 重名，修正为在新隔离目标使用 `--clean --if-exists`，并用实际备份独立重验成功。

## 未通过与部署门槛

本机 ARM64 Docker Desktop 模拟 AMD64 时，默认混合问答和单独最小 BM25 SQL 会使 PostgreSQL 进程 `signal 6 / munmap_chunk(): invalid pointer` 退出。关闭 JIT、并行或 custom scan 未解决；同版本缓存 ARM64 镜像的对照查询通过。

- [AMD64 诊断](evidence/enterprise-amd64/bm25-diagnostic.json)
- [ARM64 对照](evidence/enterprise-amd64/bm25-diagnostic-arm64.json)
- [包内目标机预检工具的本机失败结果](evidence/enterprise-amd64/database-preflight.json)

根因尚未确定，不能断言物理 AMD64 一定正常。显式向量库问答已单独通过；**交付配置没有强制关闭原生关键词索引，不将向量用例替代混合检索验收**。公司实际 OAuth、模型质量、浏览器往返、物理 x86 性能、生产备份恢复与 R5 小助手仍按各自阶段验收。

## 保留与清理

不修改上游、不删除历史数据库卷/迁移/报告、不部署公司服务器、不发布镜像或提交 Git。仅清理此次随机 project 的合成容器和卷。归档不含实例密码、公司配置、模型密钥或模型权重。
