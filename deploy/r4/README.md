# R4 原生页面运行组合

本组合将 R4 UI 和 R3 原生授权后端接在同一产品入口。实际企业部署验收仍在 R6；员工网页小助手在 R5。参见 [R4 清单](../../docs/plans/WORKSPACE_CENTRIC_R4.md)和[验收记录](../../docs/plans/WORKSPACE_CENTRIC_R4_ACCEPTANCE.md)。

R5 已完成实现与合成验收，沿用此运行组合；必须使用包含 R5 的网关、前端与 Nginx 配置，并显式设置 `MINDCREEK_EMPLOYEE_ASSISTANT_ENABLED=true`。旧镜像仅改开关无效。见[R5 发布与部署说明](../../docs/guides/R5_EMPLOYEE_ASSISTANT_ZH.md)。

## 镜像与隔离配置

- UI 使用 `images/mindcreek-ui/Dockerfile`，Node 24 测试、类型检查、构建后生成 Nginx 镜像。overlay 应用于构建副本，上游子模块不变。
- 网关使用 `images/mindcreek-gateway/Dockerfile`；原生授权配置沿用 R3，增加 Owner 只读邮箱预览。
- `scripts/r4-compose.sh` 固定使用 `mindcreek-native-r4` 项目。必须提供独立的绝对路径 `MINDCREEK_R4_ENV_FILE`、`MINDCREEK_R4_SECRET_DIR`；不复用旧业务数据库或卷。
- 用本目录 `.env.example` 建立私有配置，并按 R2 安装手册设置 admin 部署密钥；没有通用默认密码。网关、原生 App、数据库保持私有。

```sh
# 仓库根目录执行；先配置上述独立环境变量。
make r4-compose-config
./scripts/r4-compose.sh build frontend gateway
./scripts/r4-install.sh admin
./scripts/r4-install.sh default-space
./scripts/r4-compose.sh up -d
```

以上是目标环境操作说明，本次实现不自动执行部署。安装状态查询和恢复沿用[安装状态机](../r2/README.md)，但命令统一使用 `r4-install.sh`；所有权移交后不会夺回 Owner。

## 验证命令

```sh
# 使用项目要求的 Node 24，依赖从本机缓存复用。
make r4-ui-build
make r4-executable-checks
make r4-api-probe
make r4-check
```

`R4_NODE_BIN` 指定 Node 24；浏览器合成验证使用已安装的 Chrome 和 Playwright（可用 `NODE_PATH` 指向现有运行库）。API 验证新建随机 Docker 项目与临时数据库，容器端口均不公开；仅本机 Python 代理监听随机 loopback 端口供浏览器访问，结束清理该项目。所有身份、模型和文档均为合成数据。

## 页面与权限

- `/` 自动恢复员工会话或进入企业登录，首次 Viewer 加入默认空间；`/admin/login` 是本地安装 admin 入口。
- Viewer 只展示对话、历史搜索、空间/Agent/KB 选择和账户操作。Contributor 保留原生自有资源管理；原生 KB/Agent 的写接口决定最终权限。
- 成员和邀请使用原生接口；Owner 可以批量输入已有员工邮箱，预览后逐项加入。既有角色不覆盖；未开户/映射冲突逐行报告。部门目录同步尚未实现。
- 移交分为“目标成为 Owner”和“自己成为 Admin”，中断后查询现有角色恢复。平台身份与空间所有权独立。
- 原生匿名 `/embed/`、`embed.html` 和 widget 脚本入口关闭；原生公有文件链接不替代员工鉴权。
- Wiki/索引配置保留；图谱默认关闭，依赖未就绪时明确报错。真实 PDF/OCR、Wiki/图谱质量和生产模型另行验收。
