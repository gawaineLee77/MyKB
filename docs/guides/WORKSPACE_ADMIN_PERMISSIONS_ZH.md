# 空间管理员权限与 API Key

2026-10-16。适用于本次更新后的原生空间网关和前端；上游仍为未修改的 WeKnora v0.8.0。设计依据：[总体设计第 4 节](../OVERALL_DESIGN.md#4-roles-and-permissions)。验证范围见[验收记录](../plans/WORKSPACE_ADMIN_ACCEPTANCE.md)。

## 角色权限

下表中的管理操作限当前空间；知识库、智能体、共享资源和平台权限继续使用原有规则。

| 操作 | Owner | Admin | Contributor | Viewer |
|---|:---:|:---:|:---:|:---:|
| 添加、邀请、批量导入非 Owner 成员 | ✅ | ✅ | ❌ | ❌ |
| 移除非 Owner 成员 | ✅ | ✅ | ❌ | ❌ |
| 将非 Owner 成员设为 Admin / Contributor / Viewer | ✅ | ✅ | ❌ | ❌ |
| 任命、修改或移除 Owner；移交所有权 | ✅ | ❌ | ❌ | ❌ |
| 修改空间名称、描述及原生允许的空间配置 | ✅ | ✅ | ❌ | ❌ |
| 打开 API 集成界面 | ✅ | ✅ | ❌ | ❌ |
| 创建、查看、复制、修改、撤销自己的普通 API Key | ✅ | ✅ | ❌ | ❌ |
| 查看、复制、修改、撤销他人的普通 API Key | ✅ | ❌ | ❌ | ❌ |
| 管理历史归属不明的普通 API Key | ✅ | ❌ | ❌ | ❌ |
| 修改空间 API 身份与签名配置 | ✅ | ❌ | ❌ | ❌ |
| 建立或撤销空间管理授权 | ✅ | ❌ | ❌ | ❌ |

始终至少保留一个 Owner。Admin 不能对 Owner 执行成员管理，也不能通过邀请、批量导入或直接调用接口授予 Owner。内部开户/成员服务凭证受保护，Owner 可见必要元数据，但不能复制、修改或删除其密钥。停用 Key 不提供复制/编辑，只允许撤销。

Admin 的新增成员、邀请、角色修改、移除、空间名称/描述修改需要有效的空间管理授权。创建新 Key 的 Owner 和 Admin 都需要该授权；已有历史 Key 的 Owner 管理保留原生权限。授权失效时页面显示原因，由 Owner 重新授权。

## API Key 归属与范围

Admin 页面显示“我的 API Keys”，仅返回当前用户创建的 Key；猜测别人的 ID、切换空间 ID 或直接发送请求均不能访问他人的 Key。Owner 页面显示“全部 API Keys”和创建者用户 ID。旧 Key 不猜测归属，标为“历史密钥 · 归属不明”，仅 Owner 可管理。

Admin 创建的 Key 必须指定至少一个当前空间内获准的知识库，不能选择完全访问。可选能力为 `retrieve`、`chat`、`read_agents`、`ingest`、`manage_kbs`、`manage_agents`、`manage_channels`；不允许成员、空间、全局模型/存储等管理能力。原生接口仍核验能力、知识库范围和有效期。

Owner 编辑 Admin 创建的 Key 时仍保留这些限制，且不能扩大其他创建者的知识库范围。需要不同授权范围时，应由对应创建者重新创建。Owner 自己创建的 Key 可使用原有完全访问/能力配置。

创建者停用、降级、被移除或退出空间，会使其旧 Key 持久失效。重新激活、重新加入或提升角色不会恢复它们，需重新创建。API 与 MCP 均执行检查。角色修改遇到上游故障时，已经写入的 Key 失效记录保留；`workspace.member_update_failed_keys_disabled` 表示应先刷新实际成员状态，再决定是否重试，旧 Key 仍需重建。

## 安装与升级配置

本功能已纳入 2026-10-19 AMD64 完整包，R4 图谱版和 1009 版见[专用升级指南](UPGRADE_R4_AND_20261009_ZH.md)；尚未修改企业目标服务器。升级时需要同时更新产品网关和前端，并使用包含迁移 `000017_workspace_admin` 的网关执行现有迁移流程。不要仅更新 UI 后对外开放。上游 App 与数据库继续保持私网，所有外部 API/MCP 请求经过产品网关。

新版企业安装工具 `initialize` 自动生成私有文件 `instance/secrets/workspace-admin-key`，内容为 32 个随机字节的 Base64 编码，权限 `0600`；默认模板设置：

```dotenv
MINDCREEK_WORKSPACE_ADMIN_SECRET_FILE=/run/mindcreek-enterprise/workspace-admin-key
```

使用本轮新版完整包的 `bin/release upgrade` 时，工具会在原版本冷备份后自动补齐缺失的密钥及配置，保留原有有效密钥；见[两类企业实例升级说明](UPGRADE_R4_AND_20261009_ZH.md)。已配置但丢失或损坏的密钥必须从匹配备份恢复，不会自动替换。

仅在不使用新版升级工具的独立部署中，以实例文件所有者手动生成缺失密钥；将以下路径替换为真实实例目录。命令遇到已有文件会退出，不覆盖现有密钥：

```sh
python3 - <<'PY'
import base64, os, secrets
from pathlib import Path
path = Path('instance/secrets/workspace-admin-key')
with open(path, 'x', opener=lambda p, flags: os.open(p, flags, 0o600)) as output:
    output.write(base64.b64encode(secrets.token_bytes(32)).decode() + '\n')
PY
```

然后在该实例的 `enterprise.env` 增加上面的变量，使用新包的配置验证与既有升级流程。文件所有者须与 `MINDCREEK_UID` 相同，密钥目录已由企业 Compose 挂载到 `/run/mindcreek-enterprise`。独立 R3 部署可把私有密钥文件挂载到任意容器路径，并设置对应变量；它不会自动建立额外挂载。

未配置变量时网关仍可运行，但新增管理授权不可用；配置了不存在、公开可读或格式错误的密钥文件时启动失败。备份数据库时同时保留此文件。不要在已有授权上直接更换加密密钥，否则保存的会话无法解密；本次不提供在线密钥轮换流程。

## Owner 授权与恢复

1. Owner 登录并进入目标空间的“API 集成”。
2. 在“空间管理授权”点击“授权 Admin 管理”，阅读范围后确认。浏览器只提交空对象，不上传刷新凭证。
3. 网关通过原生切换空间 API 建立服务端会话，加密保存；通常按需建立仅含 `manage_members` 的内部凭证。若空间要求外部身份签名或必填身份头，则在产品权限检查后使用授权 Owner 会话执行受限成员操作，不读取签名密钥、不修改身份配置。实际管理动作记录真实操作者和请求关联 ID。
4. Admin 刷新页面后可使用获准操作；需要停止授权时，Owner 点击“撤销授权”。撤销立即阻止后续委托操作，受管 Key 的使用也依赖有效授权。

原生会话有有效期；授权 Owner 退出登录会撤销该用户的全部原生会话，授权也可能随之失效。授权者停用、失去 Owner 或会话无法刷新时，由仍有效的 Owner 重新授权。若登录后立即授权遇到 `workspace_admin.delegation_retry`，稍后重试；网关拒绝保存与浏览器相同的会话令牌。

授权撤销时上游暂时不可用，网关仍先关闭委托，再报告清理失败。恢复连接后再次撤销以清理内部凭证，或由 Owner 重新授权。新建 Key 请求结果不确定时，网关保存恢复标记，先清理未归属密钥再允许后续列表/创建，避免将它泄露成历史 Key。遇到 `workspace_admin.key_recovery_required` 应恢复上游连接后重试，不应手工删除产品元数据。

## 回滚边界

迁移只创建 `mindcreek` 产品表，不修改上游表。空的新表支持向下迁移和再次升级；存在 Key 归属、授权、撤销代次或未完成创建记录时，向下迁移会拒绝执行。

回退到不支持这些检查的旧网关之前，必须在受控维护期撤销受管的原生 Key、停止相关流量，并另行制定恢复步骤。仅保留产品表不能让旧程序执行新权限规则。不要通过删表或清空撤销记录解除迁移保护。
