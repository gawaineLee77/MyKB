# R4 AMD64：企业 Client Secret 长度校验修复

适用完整包：`r4-20260916-118b8af3`。
补丁网关：`mindcreek-gateway:r4-20260916-short-secret-3959a2e2`。

## 修复内容

取消 `MINDCREEK_IDENTITY_CLIENT_SECRET` 至少 16 个字符的产品限制。OAuth2/OIDC 都接受企业身份服务签发的非空 Client Secret；仍要求 Client ID 和 Client Secret 已填写。不需要补长、哈希或重新生成企业凭证，否则会与身份服务不一致。

此项不是本地 admin 密码，也不是产品内部的 `MINDCREEK_BROKER_CLIENT_SECRET`；后两者沿用自己的校验。补丁没有数据库迁移，不改变登录协议、成员关系或模型配置。

原完整包不可变；只重启旧镜像不会获得修复。此补丁为一个 AMD64 网关镜像，通过实例 `compose.override.json` 同时更新 gateway 和一次性 installer。原完整包和已导入的八个基线镜像仍需保留，原部署工具的镜像锁检查仍用于基线；补丁的镜像摘要由 `apply.py` 单独核对。不要将其他版本的完整包和此补丁混用。

## 1. 传输和应用补丁

将以下文件传到服务器：

```text
mindcreek-r4-20260916-short-secret-3959a2e2-amd64-hotfix.tar.gz
mindcreek-r4-20260916-short-secret-3959a2e2-amd64-hotfix.tar.gz.sha256
```

使用之前安装实例的同一账户，执行：

```sh
sha256sum -c mindcreek-r4-20260916-short-secret-3959a2e2-amd64-hotfix.tar.gz.sha256
tar -xzf mindcreek-r4-20260916-short-secret-3959a2e2-amd64-hotfix.tar.gz
cd mindcreek-r4-20260916-short-secret-3959a2e2-amd64-hotfix

# 必须指向已有实例；按实际部署路径调整。
export MINDCREEK_STATE_DIR=/srv/mindcreek/r4-instance
export MINDCREEK_PROJECT=mindcreek-enterprise-r4
python3 apply.py --package /opt/mindcreek/mindcreek-r4-20260916-118b8af3-amd64
```

脚本先校验补丁及完整包版本，再导入镜像、核对 AMD64 与镜像摘要，最后合并实例 override。已有端口、CA、网络、环境变量等配置保留；原 override 以 `compose.override.before-hotfix-*.json` 备份，权限 0600。重复执行不重复修改配置。若已有其他自定义网关镜像或非 AMD64 override，脚本拒绝替换，需要先人工核对该配置。

脚本不修改实例 `enterprise.env`，不输出或读取其中的密钥值，不自动重启企业服务。`MINDCREEK_IDENTITY_CLIENT_SECRET` 保留企业平台给出的原始值。

## 2. 继续安装或更新正在运行的实例

回到原完整包目录：

```sh
cd /opt/mindcreek/mindcreek-r4-20260916-118b8af3-amd64
python3 bin/mindcreek validate
```

### 首次安装被长度校验中断

继续原安装流程，**不要重新执行 `init` 或删除数据库卷**：

```sh
python3 bin/mindcreek install admin
python3 bin/mindcreek install status
python3 bin/mindcreek install default-space
python3 bin/mindcreek up --tls
```

使用现有企业 HTTPS 代理时，最后一条改为 `python3 bin/mindcreek up`。若安装记录显示远端结果不确定，仍按原手册核对 ID 后显式恢复；本补丁不跳过安装状态检查。

### 已完成初始化的实例

在短维护窗口替换 gateway；installer 会在下次调用安装命令时自动使用新镜像：

```sh
python3 bin/mindcreek compose up -d --no-deps --pull never --no-build --force-recreate --wait --wait-timeout 120 gateway
python3 bin/mindcreek compose restart frontend
python3 bin/mindcreek install status
python3 bin/mindcreek compose images gateway
```

重启 frontend 使其重新解析 gateway 地址。若使用包内 TLS，再执行 `python3 bin/mindcreek compose restart tls`。网关替换期间已有流式请求可能中断，完成后重新发起即可。

## 3. 确认与回退

- gateway 应为 healthy，镜像应为上述 `short-secret` 标签。
- 配置日志不再出现 `client secret of at least 16 characters`。若仍出现，检查当前 project、override 路径及实际容器镜像，重复执行补丁脚本可重新校验镜像。
- 短密钥通过本地校验后，身份服务仍会核对凭证是否正确；Token 401 等错误需要核对企业侧配置。
- 保留脚本打印的 override 备份路径。回退时，在确认没有后续 override 修改后，将该备份恢复为 `compose.override.json`，再重建 gateway、重启 frontend。若原先没有 override，只移除补丁加入 gateway/installer 的 `image`、`platform`、`pull_policy` 三个字段，保留后来添加的其他配置。恢复旧网关也会恢复原有 16 字符限制。

## 4. 本次验证范围

补丁包含独立 `evidence.json`：当前源码无缓存 Go 回归、AMD64 配置/身份测试、短密钥的启动校验对照，以及补丁合并、校验和、重复应用检查。测试只使用合成凭证。

历史 R4 浏览器/API 和原完整包报告保留为历史基线，不将它们标为本补丁已重跑。企业 OAuth 登录仍需在目标服务器验证；此前 BM25/混合检索未通过项也未由本补丁解决。
