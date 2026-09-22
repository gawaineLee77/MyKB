# R3 独立运行配置

后端/API 合成验收已完成；完整原生 UI 和目标环境验收仍在后续阶段，尚非完整产品部署版本。验收状态见 [R3 记录](../../docs/plans/WORKSPACE_CENTRIC_R3_ACCEPTANCE.md)。

R3 沿用 [R2 初始化与恢复](../r2/README.md)，增加原生授权运行组合。使用独立环境文件、安装密钥目录、数据库和卷；不得指向旧业务数据。`scripts/r3-compose.sh` 固定使用 `mindcreek-native-r3` 项目及单独模型配置输出，不复用 R2 项目卷。

```sh
MINDCREEK_R3_ENV_FILE=/absolute/path/to/r3.env \
MINDCREEK_R3_SECRET_DIR=/absolute/path/to/r3-secrets \
  ./scripts/r3-compose.sh config --quiet
```

环境结构以 R2 示例为基础；真实配置及密钥不提交。需构建完成 R3 的网关镜像，不能使用旧镜像冒充 R3。合成验证另用随机项目、内部网络与缓存镜像，验证后仅移除该次创建的容器和卷。

新运行组合固定关闭个人笔记及发布订阅，使用原生四角色和资源权限。图谱默认关闭；OAuth2、本地 admin、首次 Viewer 开户及默认模型保持。原生页面和菜单在 R4 交付。

## 安装与恢复入口

独立参数结构见 [.env.example](.env.example)。R3 网关以现有 gateway Dockerfile 构建并设置 `MINDCREEK_R3_GATEWAY_IMAGE`；R2 登录 UI 可用于接口验收，原生业务 UI 尚待 R4，不能据此宣布完整产品可上线。

设置 `MINDCREEK_R3_ENV_FILE`、`MINDCREEK_R3_SECRET_DIR` 后，按 R2 手册的密钥权限与封闭安装窗口要求执行：

```sh
./scripts/r3-install.sh status
./scripts/r3-install.sh admin
./scripts/r3-install.sh default-space
```

恢复使用同一脚本的 `admin --admin-id ID`、`default-space --default-space-id ID --member-key-id ID`、`repair-member-key --member-key-id ID`。内部复用 R2 状态机，但所有 Compose 操作固定落在 R3 项目；不调用旧 R2 项目。日常 admin 登录仍通过 `/admin/login` 与网关认证接口。

迁移 000015 自动前向创建主体会话绑定和访问审计。空表可以受控回退；已有绑定或审计时拒绝降级，不删除记录以强行回退。恢复应保留 R3 数据库并恢复对应网关，或在明确失败结果后前向修复。不得把旧业务数据卷挂到 R3 新安装中。

## 功能与限制

- 所有新资源写操作由原生接口授权；发布订阅/笔记/grant/profile 接口统一 410 `feature.retired`，未知接口拒绝。
- 默认模型只补缺省项；显式不可用模型报错。Wiki 和索引参数保留；图谱默认关闭，开启需同步能力配置、网关开关和原生 Neo4j 依赖；真实抽取验收后置。
- MCP 接受员工/admin bearer 或受限 Key；建议只授予所需的 `retrieve`、`chat`、`read_agents` 和 KB 范围。工具发现也需认证；Key 与员工会话分开，不能在网页公开 Key。
- 历史关键词搜索可用；历史向量搜索、隐式 Agent 工具和同 ID 来源歧义的错误及提案见 [R3 接口缺口](../../docs/plans/WORKSPACE_CENTRIC_R3_UPSTREAM_GAPS.md)。

## 可重复验证

`make r3-api-probe` 构建当前网关并运行隔离 API/MCP + R2 回归；`R3_NODE_BIN=/absolute/path/to/node24 make r3-executable-checks` 执行 Go、竞态、Node 24、Python 3.12 和静态检查；`make r3-check` 核验源码指纹、证据和文档。不会连接公司身份或真实模型；实际部署仍需 R4–R6 的对应验收。
