# R4 企业文档链接配置

## 1. 行为

现有 R4 镜像没有统一文档链接开关，需要先升级一次前端到 `mindcreek-ui:r4-20260918-links1`。后续通过实例目录的 `ui-links/links.json` 控制，无需重新编译或重建镜像。

默认 `enabled: false`，隐藏所有纳入管理的导航入口；启用总开关后，只显示配置了有效地址的入口。空值、缺少配置、读取失败、格式错误时隐藏，不回退到 WeKnora/GitHub 地址。页面每次刷新重新读取；已打开页面不自动轮询。

| 配置键 | 入口 |
|---|---|
| `help` | 左下角“帮助与文档” |
| `project` | 左下角“项目主页”（原 GitHub 入口，去掉 GitHub 图标与 Star 提示）；旧登录组件的信息入口 |
| `rbac` | 成员管理“了解 RBAC” |
| `builtinModels` | 模型管理“查看内置模型管理指南” |
| `api` | API 集成的 API 文档行及“打开文档” |
| `knowledgeGraph` | 知识图谱未启用时的说明入口 |
| `migration` | 系统信息的数据库迁移排障文档 |
| `feedback` | 系统信息的故障反馈入口；不向地址自动追加错误内容或环境信息 |
| `integration` | 原生渠道集成说明（该功能在当前运行组合中仍关闭） |
| `sandbox` | 原生沙箱说明（该功能在当前运行组合中仍关闭） |

此配置管理产品自带的上游文档/项目链接。用户文档、对话引用中的 URL 和第三方供应商控制台链接不由此配置改写；模型 API 地址也不受影响。配置只控制入口显示，不改变功能、角色或后端权限。

## 2. 离线升级一次前端

使用补丁包 `mindcreek-r4-20260918-links1-amd64-addon.tar.gz`，适配原完整企业包 `mindcreek-r4-20260916-118b8af3-amd64`。可与已有网关登录补丁和 graph1 扩展并用；只更新 frontend 镜像及文档链接配置挂载。

以下路径与项目名按正在使用的部署填写，沿用原实例，不重新初始化：

```sh
sha256sum -c mindcreek-r4-20260918-links1-amd64-addon.tar.gz.sha256
tar -xzf mindcreek-r4-20260918-links1-amd64-addon.tar.gz
cd mindcreek-r4-20260918-links1-amd64-addon

export MINDCREEK_STATE_DIR=/srv/mindcreek/r4-instance
export MINDCREEK_PROJECT=mindcreek-enterprise-r4
python3 configure.py --package /opt/mindcreek/mindcreek-r4-20260916-118b8af3-amd64
```

工具校验补丁及基线、加载并核对 AMD64 镜像、备份原 override、合并 frontend 配置，并在尚不存在时创建 `ui-links/links.json`。已有 OAuth、CA、模型、网关、Neo4j、端口和业务数据配置保留。重复执行保留已填写的链接。遇到自定义前端镜像或冲突挂载时停止，避免覆盖自定义修改。此命令不会自动重启服务。

随后进入原完整包目录，仅重建 frontend：

```sh
cd /opt/mindcreek/mindcreek-r4-20260916-118b8af3-amd64
python3 bin/mindcreek validate
python3 bin/mindcreek compose up -d --no-deps --pull never --no-build --force-recreate --wait --wait-timeout 120 frontend
python3 bin/mindcreek compose ps
```

浏览器刷新后，五个已报告入口及表中其他同类入口默认隐藏。无需升级 app/gateway，不重建账户、不执行数据库迁移。

## 3. 将来启用内部文档

编辑 `$MINDCREEK_STATE_DIR/ui-links/links.json`，例如只启用四个文档入口：

```json
{
  "version": 1,
  "enabled": true,
  "links": {
    "help": "https://docs.company.internal/mindcreek/",
    "rbac": "https://docs.company.internal/mindcreek/roles",
    "builtinModels": "https://docs.company.internal/mindcreek/models",
    "api": "https://docs.company.internal/mindcreek/api",
    "project": ""
  }
}
```

保留文件权限 `0644` 和 `ui-links` 目录权限 `0755`，让容器中的 Nginx 可以读取。配置通过目录只读挂载，支持编辑器原子替换文件；保存后刷新浏览器即可，不必重启容器。可先运行 `python3 -m json.tool "$MINDCREEK_STATE_DIR/ui-links/links.json" >/dev/null` 检查 JSON 格式。

- `enabled: false`：所有受管入口隐藏。
- `enabled: true`：仅显示 URL 有效且非空的项；未填写的键保持隐藏。
- 支持完整 `https://` / `http://` 地址，或从 `/` 开始的同域路径；同域路径需由企业反向代理实际提供文档，配置本身不会托管文档。
- 拒绝 `javascript:`、`data:`、带用户名密码的 URL、`//host` 和反斜杠等地址。不自动判断某个域名是否属于内网，部署者应只填写批准的内部地址。
- 配置通过主站 `/mindcreek-links.json` 公开读取，内容仅放导航 URL，不放密码、Token、签名访问链接等秘密。文档访问控制由内部文档服务负责。

`/mindcreek-links.json` 使用 `Cache-Control: no-store`。若经过企业 CDN/反向代理，保留此策略，避免缓存旧配置。旧的 `VITE_KG_GUIDE_URL` 不再控制企业图谱说明入口，统一使用 `knowledgeGraph`。

## 4. 检查与回退

验证：未配置时按钮消失；只配置 RBAC 时仅恢复“了解 RBAC”；启用内部地址后实际打开正确页面；Viewer 的管理菜单仍保持隐藏。专项证据见[验收记录](../plans/R4_UI_LINKS_ACCEPTANCE.md)。

隐藏所有入口只需将 `enabled` 改为 `false` 并刷新，无需回退镜像。若需回退整个前端，使用部署前记录的 frontend 镜像，移除该服务的 `mindcreek-runtime` 挂载，再按上面的命令重建 frontend。工具生成的 `compose.override.before-ui-links-*.json` 用于对照；如果后续已做其他配置变更，不要直接覆盖整个 override。回退旧前端会恢复原来的上游链接行为。
