# R4 企业文档链接配置验收

## 1. 结论与交付边界

2026-09-18：实现及本地合成验收完成。镜像 `mindcreek-ui:r4-20260918-links1` 默认隐藏产品自带的 WeKnora 文档/项目入口；部署者可配置内部 URL，保存并刷新页面后逐项启用。使用[部署与配置指南](../guides/R4_ENTERPRISE_LINKS_ZH.md)中的离线补丁包，仅升级前端一次。

上游固定为 WeKnora v0.8.0（`1edcd54b43606d9079bb36650efe3f68707a79ea`），子模块源码无修改。实现位于产品 UI overlay、前端 Nginx 模板和安装辅助工具。不涉及数据库迁移、权限变更、OAuth 或模型接口修改。历史 R2–R4/图谱验收记录和旧镜像包保留。

## 2. 实现范围

| 项目 | 实际行为 |
|---|---|
| 用户报告入口 | 帮助与文档、项目主页（原 GitHub）、了解 RBAC、内置模型指南、API 文档全部由配置控制 |
| 其他同类入口 | 图谱帮助、迁移排障、故障反馈、原生集成/沙箱说明、旧登录组件项目链接同时接入 |
| 默认与异常 | 总开关默认关闭；空值、缺失文件、错误 JSON、网络失败均隐藏，最多等待 2 秒后继续启动 UI；无上游地址回退 |
| 启用方式 | `enabled: true`，为具体键填写 HTTP(S) 或同域根路径；未填项仍隐藏 |
| 安全与角色 | 拒绝可执行 scheme、协议相对地址、URL 用户名密码和反斜杠；不更改后端授权；Viewer 原有导航限制保留 |
| 反馈链接 | 不自动向 URL 追加诊断信息或错误正文 |
| 运行时文件 | 实例 `ui-links/links.json` 只读目录挂载；`/mindcreek-links.json` 无缓存读取，支持文件原子替换，无需重启 |
| 升级工具 | 校验包与原 R4 基线；导入并核对 AMD64 镜像；锁定安装、备份并合并 frontend override；重复执行保留已有配置 |

未纳入范围：用户内容/引用链接、第三方供应商控制台地址、内部文档服务本身及其访问控制。配置为公开导航数据，不能存放秘密。

## 3. 本轮验证

| 检查 | 结果与证据 |
|---|---|
| Node 24 前端 | 598 项测试通过；`vue-tsc --build` 与生产 Vite 构建通过；构建仍有体积警告 |
| 浏览器 | 7 场景通过：原界面、默认隐藏、全部启用、单项启用、404、错误 JSON、Viewer；真实编译产物＋合成 API，未访问外部服务。链接 href、帮助弹窗目标、无诊断查询参数和 Viewer 路由检查通过；[报告](evidence/ui-links-browser.json) |
| 实际 AMD64 Nginx | 配置语法、默认隐藏、no-store、挂载读取、原子更新、删除挂载文件后回退隐藏共 6 项通过；[报告](evidence/ui-links-nginx.json) |
| Python 3.12 | 4 项测试通过：配置保留/幂等、自定义冲突拒绝、文件权限与原子写入、classic/containerd 镜像 ID 契约 |
| 实际打包工具 | 并发/重复执行、已有链接保留、其他服务与 env 保留、单个私有备份、文件权限、原完整包 Compose 渲染通过；未启动业务服务；[报告](evidence/ui-links-apply.json) |
| 工程边界 | `make stage1-check`、中英文设计结构/文档链接、overlay 锚点与默认隐藏检查、本次新增文件及修改行的空白检查通过；[源码和日志摘要](evidence/ui-links-release.json) |

新前端测试、构建、镜像和证据来自本轮源码；旧报告不替代上述验证。后端源码未更改，本轮未重复运行 Go/OAuth/图谱业务测试。

## 4. 截图

19 张截图均已检查。启用态使用合成内部地址；模型“不可用”和系统“迁移失败”来自用于展示文档入口的模拟响应，不代表真实环境健康状态。

| 区域 | 修改前 | 默认隐藏 | 配置启用 |
|---|---|---|---|
| 用户菜单 | [前](../assets/ui-links/before-menu.png) | [隐藏](../assets/ui-links/hidden-menu.png) | [启用](../assets/ui-links/enabled-menu.png) |
| 成员管理 | [前](../assets/ui-links/before-members.png) | [隐藏](../assets/ui-links/hidden-members.png) | [启用](../assets/ui-links/enabled-members.png) |
| 模型指南 | [前](../assets/ui-links/before-models.png) | [隐藏](../assets/ui-links/hidden-models.png) | [启用](../assets/ui-links/enabled-models.png) |
| API 文档 | [前](../assets/ui-links/before-api.png) | [隐藏](../assets/ui-links/hidden-api.png) | [启用](../assets/ui-links/enabled-api.png) |
| 系统排障 | [前](../assets/ui-links/before-system.png) | [隐藏](../assets/ui-links/hidden-system.png) | [启用](../assets/ui-links/enabled-system.png) |
| 图谱帮助 | [前](../assets/ui-links/before-graph.png) | [隐藏](../assets/ui-links/hidden-graph.png) | [启用](../assets/ui-links/enabled-graph.png) |

另有 [Viewer 菜单](../assets/ui-links/after-viewer-menu.png)；原生集成/沙箱当前未开放，通过 overlay 及编译检查，未作为可访问页面进行浏览器验收。

## 5. 部署后核对

尚未在企业服务器部署；AMD64 Nginx 和 Python 测试运行于本机 ARM64 Docker 仿真。classic Docker 的 ID 形式通过单元测试验证，未在第二台 classic 存储服务器实测。内部文档可达性、企业代理缓存策略和企业 OAuth 实际登录仍需目标环境核对。

按指南更新 frontend 后，确认未配置时入口隐藏；填写一个内部地址后刷新并打开；检查原有登录与问答正常。修改配置不需要重建镜像。撤回链接配置可直接设置 `enabled: false`；整体镜像回退方法见指南。
