# R4 图谱版与 20261009 企业实例升级

更新：2026-10-19。目标为 `mindcreek-current-20261019-d3fafdaf-amd64` 完整离线包，包含产品迁移 `000021`。两条直接升级及升级前冷备份的原版恢复均已演练通过，详情见[包验收](../plans/CURRENT_AMD64_UPGRADE_20261019.md)；尚未操作企业服务器。请核对交付包 `RELEASE.json`、`evidence/amd64-smoke.json` 及两条升级证据。

## 1. 两类来源使用同一个新包

| 当前部署 | `--from-package` 指向 | 升级方式 |
|---|---|---|
| R4 + 图谱补丁，可能另有身份/链接补丁 | 原 `mindcreek-r4-20260916-118b8af3-amd64` **完整包目录** | 直接升级；自动解除已识别的旧产品镜像固定，保留图谱卷、密码及 UI 链接配置 |
| 1009 版本 | 原 `mindcreek-current-20261009-a86981ad-amd64` 完整包目录 | 直接升级；保留已有 R5、图谱及业务数据 |

两者均为 Linux AMD64，使用固定 WeKnora v0.8.0 及相同基础镜像。无需安装旧 Phase 5，也无需先把 R4 升到 1009。`--from-package` 不能指向图谱补丁目录。原完整包、补丁和备份继续保存；未知自定义镜像或外部 Neo4j 配置会被工具拒绝，需要单独审查。

## 2. 升级前必须知道的权限变化

| 内容 | 升级后的行为 | 开放流量前的操作 |
|---|---|---|
| 旧知识库 | 按可信创建者初始化为受限；创建者与空间 Owner/Admin 可见，未知创建者仅管理者可见 | Owner 逐空间列出历史库并读取“访问与协作”，核对数量与创建者，明确公开或指定成员 |
| 文档/分块 | 可见和问答不自动授予下载或写入 | 按需要逐项授予七类文档与分块操作 |
| 旧 API Key | 未建立产品范围的 Key 拒绝知识访问 | Owner 核对后保存明确知识库范围或重建，再验收外部调用 |
| Admin 管理 | Admin 可管非 Owner 成员和自己的 Key；委托操作需要 Owner 授权 | Owner 在 API 集成启用空间管理授权 |
| 协作编辑 | 内容协作采用独立执行授权 | Owner 在空间设置启用知识库协作写入 |
| 旧 Agent/空间记忆 | 不兼容的写工具、外部 MCP/Skills、沙箱和记忆不能用于新权限问答 | 关闭空间记忆，核对并重新保存兼容 Agent；旧沙箱会话需新建 |
| 全站外观 | 初始名称“小纳”，可以修改；使用内置 Logo 回退 | 安装本地管理员在“设置 → 服务外观”修改名称、上传自己的 Logo |

升级不会自动公开历史库、猜测旧 Key 的访问范围或替 Owner 建立委托。这些变化是权限收紧，不表示原文档被删除。细节见[知识库权限](KNOWLEDGE_BASE_PERMISSIONS_ZH.md)、[空间管理权限](WORKSPACE_ADMIN_PERMISSIONS_ZH.md)和[外观/Wiki MCP](SERVICE_APPEARANCE_AND_WIKI_MCP_ZH.md)。

机器 Key 本期不提供知识库创建/删除、知识库配置或 ACL 管理权限。旧自动化若依赖这些接口，仅重新保存 Key 范围仍不能恢复此类调用；需先由有权用户创建和配置知识库，再让机器在明确授权的库内检索或执行获准的文档操作。

## 3. 每个实例独立执行

先在服务器核对新包的外层 SHA256，解压并进入新包目录。以下大写路径均替换为该实例的真实值，不能把两套实例的目录、项目名混用。

```sh
cd /NEW_PACKAGE_DIRECTORY
python3 bin/mindcreek verify
python3 bin/mindcreek load

export MINDCREEK_STATE_DIR=/EXISTING_INSTANCE_DIRECTORY
export MINDCREEK_PROJECT=existing-compose-project

python3 bin/release plan --from-package /OLD_FULL_PACKAGE_DIRECTORY
```

`plan` 只读，核对源版本、目标版本、卷名、配置补齐和待验收权限。两套实例分别保留输出。项目名和实例目录从原运维记录核对，不能使用新名称；新名称可能指向另一组卷。原包目录缺失时先恢复对应可信完整包，不伪造版本记录。

先在物理 AMD64 服务器运行随包 `bin/check-database` 隔离检查；它不操作企业数据库：

```sh
python3 bin/check-database --output /SAFE_OUTPUT_DIRECTORY/database-check.json
```

进入维护窗口，暂停员工访问、外部 API/MCP、导入与其他写入。备份目录须不存在，并有足够容量保存所有卷及旧产品镜像。以原实例文件所有者执行：

```sh
python3 bin/release upgrade \
  --from-package /OLD_FULL_PACKAGE_DIRECTORY \
  --backup /SAFE_BACKUP_DIRECTORY/before-upgrade-instance-date

python3 bin/mindcreek ps
python3 bin/mindcreek install status
```

不要重新执行 `init`、首次安装、旧图谱 `graph.py enable`，不要运行 `down -v`。工具保留企业 OAuth、模型、CA、JWT/AES、原管理员和默认空间；新增委托加密文件只补缺失项，不替换已有密钥。

## 4. 验收后再开放流量

1. 确认安装状态 `ready`、所有启用的服务健康、旧文档/会话/图谱存在；身份和模型设置保持正确。
2. 按第 2 节完成每个空间的旧库、API Key 和 Agent 盘点。`active-release.json` 中的 `started_requires_access_review` 表示仍需运维执行验收，不能仅看容器健康即开放。
3. 使用 Owner、Admin、两个不同编辑和 Viewer 验证私有/指定/公开知识库，上传、修改、删除、下载、分块权限与撤销。验证外部 Key 仅检索已授权知识库，Wiki MCP 为 `wiki_search`、`wiki_read_page`。
4. 验证企业登录、真实模型问答、引用、文件预览、图谱、智能体图片及全站名称/Logo；如启用了 R5，验证发布渠道与历史记录。
5. 保留验收记录后开放该实例流量。建议先升级一套并完成业务验收，再升级另一套。

升级成功后的备份必须同时保留数据库与 `secrets/workspace-admin-key`；直接更换该密钥会使保存的委托会话无法解密。

## 5. 失败与回退

工具不自动切回旧网关。失败时保持停写，检查新包日志和 `upgrade-pending.json`，不要删除 ACL、授权或撤销表来强制降级。

需要回退时，使用升级前**对应旧完整包和成套冷备份**，恢复到新的隔离实例，验证后切换企业入口。具体命令见[隔离恢复](ENTERPRISE_DEPLOYMENT_AND_UPGRADE_ZH.md#72-恢复到新的隔离实例)。旧网关不执行新 ACL，不能连接升级后的数据库直接对外开放；新权限配置和备份后写入也不会自动迁回旧版。
