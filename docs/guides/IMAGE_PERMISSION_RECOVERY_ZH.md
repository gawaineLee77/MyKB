# 图片权限修复与升级

适用于 `mindcreek-current-20261020-699031ce-amd64` 图片权限修复完整包（产品迁移 `000022`）。新安装、R4 图谱版、1009、1019 的升级和冷恢复验证见[本次验收](../plans/CURRENT_AMD64_IMAGE_FIX_20261020.md)。旧版 `current-20261019-d3fafdaf` 的知识库图片校验默认返回 409，问答的消息图片出口也被关闭；这并不表示存储卷中的图片被删除。

后续合并包 `current-20261020-699031ce-104658` 包含相同的图片修复及模型缓存修复；尚未升级时，可直接按[模型与图片合并修复说明](MODEL_REFRESH_FIX_ZH.md)升级，本页的图片检查与归属确认步骤仍适用。

## 修复后的行为

- 当前空间 Owner/Admin 可以读取经本空间资源登记校验的知识库图片；该例外不开放其他用户的私有会话附件。平台管理员、知识库创建者和 API Key 不会因此获得空间管理者例外。
- 创建者、获准编辑和访客读取图片时，使用服务端解析记录核验“图片 → 文档 → 知识库”，并检查当前可见权限。带多模态解析记录的历史图片自动恢复，无需重建知识库或重新调用模型。
- 问答图片先核验当前用户拥有该会话，再检查图片所属知识库属于会话范围且当前仍可见。用户本人上传的图片使用服务端保存的消息附件记录核验。
- 旧正文中的已登记存储路径转为不透明资源句柄，再经过上述鉴权。编辑正文、复制链接、修改 `image_info` 都不能建立图片访问权限。
- 浏览器重新聚焦或收到权限变更事件时，会清理图片缓存并重新鉴权。已被用户另行保存的内容无法追回。

资源登记缺失、复制产生但无解析记录、未启用多模态处理等图片不能仅凭正文链接自动确认归属。Owner/Admin 仍可读取符合资源登记和空间校验的图片；普通成员需要下面的人工核验流程。原文件下载和预览仍按既有下载权限执行。

## 升级

使用新完整包，不覆盖旧包目录。核对外层 SHA256 后解压，进入新包目录。保持原实例目录和 Compose 项目名：

```sh
export MINDCREEK_STATE_DIR=/EXISTING_INSTANCE_DIRECTORY
export MINDCREEK_PROJECT=existing-compose-project

python3 bin/mindcreek verify
python3 bin/mindcreek load
python3 bin/release plan --from-package /OLD_FULL_PACKAGE_DIRECTORY
```

确认计划后进入维护窗口，暂停写入，再执行：

```sh
python3 bin/release upgrade \
  --from-package /OLD_FULL_PACKAGE_DIRECTORY \
  --backup /SAFE_BACKUP_DIRECTORY/before-image-permission-fix
python3 bin/mindcreek ps
python3 bin/mindcreek install status
```

备份目录必须尚不存在。不要执行 `init`、删除卷、清空知识库或批量重解析。升级停止容器后，冷备份大卷可能耗时较长；不要同时再次启动升级。

验收同一张历史图片的文档预览和问答显示，至少分别用 Owner/Admin、创建者、获准成员、未获准成员测试。撤销成员可见权限后重新聚焦浏览器，确认图片不再加载。只有容器健康并不足以证明业务验收通过。

## 检查仍未恢复的图片

在新包目录、保留上面的环境变量，以原实例运维身份执行。`--tenant` 是空间数字 ID，不是知识库 UUID 或 Compose 项目名：

```sh
python3 bin/image-resources audit --tenant 123 > /SAFE_OUTPUT_DIRECTORY/images-page1.json
```

该操作只读。`verified`、`unverified` 是本页扫描结果，`total` 是整个空间登记的有效图片数量；若 `truncated=true`，使用返回的 `next_cursor` 继续：

```sh
python3 bin/image-resources audit --tenant 123 --after RETURNED_CURSOR \
  > /SAFE_OUTPUT_DIRECTORY/images-page2.json
```

`processing_span` 表示解析记录证明归属，`operator_review` 表示安装运维人员已确认。`no_trusted_knowledge_provenance` 表示缺少可信文档关联，不等于图片文件不存在。检查工具不读取模型密钥，也不会上传资料；结果包含内部资源和文档标识，应留在企业内部。

## 对缺少记录的图片确认归属

仅对已独立核对原文档与图片的条目执行。不能直接把整个检查报告作为确认文件，也不能将正文中出现链接视为核验完成。可用 Owner/Admin 查看图片并与原文件、原始导入记录核对。

创建精简 JSON 文件，每项使用检查报告中的 `reference` 和 `fingerprint`，填写真实所属**文档 ID**和核验说明：

```json
{
  "tenant_id": 123,
  "items": [
    {
      "reference": "resource://REPLACE_WITH_REAL_HANDLE",
      "knowledge_id": "REPLACE_WITH_DOCUMENT_ID",
      "fingerprint": "REPLACE_WITH_AUDIT_FINGERPRINT",
      "reason": "已与原始文件及图像位置逐项核对"
    }
  ]
}
```

```sh
python3 bin/image-resources apply --tenant 123 \
  --confirm-reviewed --manifest /SAFE_OUTPUT_DIRECTORY/reviewed-images.json
```

工具只写产品归属表，不修改原生文档、分块、资源记录或图片文件。跨空间文档、已删除图片和失效指纹会被拒绝。逐项操作可重复；中途失败时会报告已经应用的数量，核对后可再次提交同一文件。确认后仍需知识库可见权限，后续移除成员、删除文档或撤销可见权限会继续生效。

若资源登记本身不存在，或者源图片文件确实缺失，此工具不会凭空重建图片。应依据原备份和检查结果另行恢复，不能删除权限记录来绕过校验。

## 回退

保留旧完整包与整套升级前冷备份。`000022` 在存在人工确认记录时拒绝破坏性回滚；需要回退时按原包的隔离恢复流程恢复完整实例，不将旧网关连接到已经升级的数据库。
