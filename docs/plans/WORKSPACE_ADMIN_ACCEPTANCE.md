# 空间管理权限扩展验收

日期：2026-10-16。范围：[批准设计](../OVERALL_DESIGN.md#4-roles-and-permissions)、[操作与部署说明](../guides/WORKSPACE_ADMIN_PERMISSIONS_ZH.md)。本次是仓库实现与合成验证，不包含发布镜像、企业服务器部署或真实企业 OAuth 联调。

## 实现

- 产品 `workspaceadmin` REST 适配、网关路由及前端构建覆盖；四角色不变，上游源码不变。
- Admin 管理非 Owner 成员、邀请、批量导入、空间名称/描述；Owner 管理及移交规则保持至少一名 Owner。
- API 集成入口对 Owner/Admin 开放；Admin 仅管理自己的 Key，Owner 管理全部普通 Key；历史 Key 仅 Owner 可见。
- Owner 明确授权、会话加密、严格请求白名单、内部凭证保护、创建者归属、审计与不确定创建恢复。
- API/MCP 检查持久撤销；停用/恢复、成员降级/重新提升、移除/重新加入不能复活旧 Key。人类与机器成员修改共用空间锁，身份状态修改单独串行。
- 兼容空间原生 tenant、可选/必填 direct_header 和 signed_token 模式；机器成员操作保留原有身份校验头。
- `000017` 产品迁移及安装密钥模板；活跃权限元数据阻止破坏性向下迁移。

## 执行证据

测试在本地合成环境执行，命令与源码摘要见[检查清单](evidence/workspace-admin-checks.json)。编译 UI 的合成接口测试不替代真实后端或企业 OAuth 验收。

| 验证 | 状态 |
|---|---|
| `make phase1-gateway-test` 网关全量 Go 测试 | 已通过 |
| 权限服务、身份、成员预览、HTTP 路由及上游 REST 适配 `go test -race` | 已通过；HTTP 专项 8 组 / 35 个用例 |
| PostgreSQL 17 实库升级、重复升级、空库回滚/再升级、带状态回滚拒绝、锁取消及连接池饱和并发 | 已通过，独立临时合成数据库 |
| 企业安装器私有随机密钥与兼容配置 | 已通过，11 项测试 |
| 前端单元测试、类型检查、生产构建 | 已通过，611 项测试；撤销授权修复后类型检查/构建及浏览器重验通过 |
| 编译 Vue 的浏览器场景与前后截图 | 已通过，8 场景、19 张截图、无意外外部请求 |
| `make stage1-check`、`make r4-check`、`make phase5-check` | 已通过；包括继承的路由、模型、身份、检索和 MCP 契约 |
| 上游子模块干净 | 已确认，固定 v0.8.0，无源码补丁 |

浏览器用例覆盖 Owner/Admin 成员页、空间信息、Key 列表/创建、Contributor/Viewer 入口拒绝、缺少授权及撤销后状态刷新。所有 API 响应使用合成固定数据，不连接生产账户或私有文档。完整清单见[浏览器报告](evidence/workspace-admin-browser.json)。

| 界面 | 修改前 | 修改后 |
|---|---|---|
| Admin API 集成 | [权限不足](../assets/workspace-admin/before-admin-api-keys.png) | [我的 API Keys](../assets/workspace-admin/after-admin-api-keys.png) |
| Owner API 集成 | [原列表](../assets/workspace-admin/before-owner-api-keys.png) | [全部 Key 与归属](../assets/workspace-admin/after-owner-api-keys.png) |
| Admin 成员管理 | [原界面](../assets/workspace-admin/before-admin-members.png) | [非 Owner 管理](../assets/workspace-admin/after-admin-members.png) |
| Admin 空间信息 | [原界面](../assets/workspace-admin/before-admin-space-config.png) | [可编辑](../assets/workspace-admin/after-admin-space-config.png) |

附加执行 `make r5-check` 会报告原 R5 发布证据的源码指纹与截图评审已过期：本次网关/前端源码发生变化，旧发布证据不适用于新源码。本次未修改或冒用旧 R5 证据；当前实现由上面的测试和本专项截图验证，新发布包仍需重新生成整包 R5 验收。

## 发布后验证

正式发布时还需在目标环境验证迁移和密钥挂载、真实 Owner 授权与刷新/退出、Admin 非 Owner 成员操作和自有 Key、跨用户/跨空间拒绝、身份停用及 API/MCP 失效。已有历史 R4/R5 发布包和证据不会因为本次源码变更自动获得这些能力。
