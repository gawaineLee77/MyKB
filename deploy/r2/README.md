# R2：全新企业安装与恢复

本配置启用产品网关的本地 admin、默认空间和员工开户。尚未执行真实企业部署；合成验收见[验收记录](../../docs/plans/WORKSPACE_CENTRIC_R2_ACCEPTANCE.md)。R3–R4 的授权替换、旧功能退役和原生知识库界面恢复仍独立实施。

## 1. 准备独立配置

将 [.env.example](.env.example) 复制到部署主机的受保护目录，替换数据库/JWT/AES/Redis、企业 OAuth2 和托管模型配置；占位符不能用于部署。账号密码只写入 `admin-password` 密钥文件，不提供默认密码。

```sh
export MINDCREEK_R2_ENV_FILE=/absolute/protected/enterprise.env
export MINDCREEK_R2_SECRET_DIR=/absolute/protected/enterprise-secrets
export MINDCREEK_R2_PROJECT=mindcreek-enterprise
```

密钥目录权限为 `0700`，文件为 `0600`，禁止符号链接；配置中的 `MINDCREEK_R2_UID/GID` 必须匹配密钥目录所有者。通过部署密钥工具写入足够强的 admin 密码，文件名 `admin-password`。密码不要放入命令行参数、Git、报告或浏览器存储。

OAuth 字段沿用[身份配置指南](../../docs/guides/PHASE5_IDENTITY_PROVIDER.md)，以企业实际契约填写。模板模型只用于合成测试，实际部署替换为批准的默认模型。配置渲染只写环境变量引用，不写模型密钥值。

## 2. 分步安装

从仓库根目录执行：

```sh
make r2-compose-config
scripts/r2-compose.sh build gateway frontend
scripts/r2-install.sh admin
scripts/r2-install.sh status
scripts/r2-install.sh default-space
scripts/r2-install.sh status
scripts/r2-compose.sh up -d
```

`admin` 启动私有依赖，在私有 App 短暂开放注册，创建 tenantless admin，再以原生 bootstrap 配置提升平台管理员，关闭注册并核验身份。退出时重建 App，移除 bootstrap 配置。安装窗口不启动前端；安装未完成时网关拒绝员工 OAuth 和知识访问。中断或掉电后重新执行 `admin` 完成恢复，不能直接开放员工入口。

`default-space` 使用 admin 的人类 bearer 建空间并成为 Owner，保存固定 ID，设置 tenantless/邀请模式和内部建空间能力，创建只含 `manage_members` 的凭证，写入 `member-key`，核验后标记 `ready`。原生系统设置需要有效空间，因此在建空间后写入；此前由环境变量关闭密码注册。

默认只发布 `127.0.0.1:18080` 前端端口。App、网关、数据库、Redis、DocReader 不发布主机端口；项目专用卷不复用历史 Phase 0–5 卷。TLS/域名代理只连接前端，日常管理员入口为 `/admin/login`。无需使用应急 App 端口。正常主站 `/` 恢复空间会话，或自动进入企业登录。

## 3. 恢复操作

安装状态可用 `scripts/r2-install.sh status` 读取；Web 状态接口只允许安装 admin。命令输出只有非敏感标识和阶段。

| 状态/故障 | 显式恢复及约束 |
|---|---|
| `registering`，建号结果不明 | 核对内部账号 ID、配置邮箱及密码后执行 `scripts/r2-install.sh admin --admin-id ID`；不会按同名邮箱接管陌生账号 |
| 已存在其他平台管理员 | 原生 bootstrap 拒绝自动提权；停止安装，核对是否误用了非空数据库，不直接改表提权 |
| `space_creating`，建空间结果不明 | 确认指定空间确由安装 admin 拥有，执行 `scripts/r2-install.sh default-space --default-space-id ID`；不按名称猜测或再次创建 |
| `key_creating`，密钥创建/写文件中断 | 当前 Owner 核对原生 API Key ID、作用域与密钥值，安全写入 `member-key`，执行 `scripts/r2-install.sh default-space --member-key-id ID` |
| 已就绪，成员密钥丢失/撤销 | 当前默认空间 Owner 创建仅有 `manage_members` 的替代凭证，原子替换 `member-key`；临时安全写入自己的 bearer 到 `recovery-owner-bearer`，执行 `scripts/r2-install.sh repair-member-key --member-key-id ID`，随后删除临时 bearer 文件 |
| 开户写入结果不明 | 先由 Owner 核对实际成员；已有成员时重试会完成进度。若未加入但结果不明，Owner 显式添加该员工 Viewer 后再重试；不盲目重复成员写入 |
| 员工曾被移除 | 再次登录不会自动加入；只有 Owner 显式添加/邀请可恢复成员，已有其他空间可继续使用 |

凭证恢复命令核验当前 Owner、固定空间、Key ID、密钥一致性、唯一 `manage_members` 作用域及实际可用性。允许移交后的新 Owner 恢复，不赋予旧 admin Owner，也不改变平台管理员 ID。已完成安装的重复运行不重设密码、重建空间或重夺所有权。修改 admin 密码后，后续确需密码的维护操作要同步更新部署密钥文件。

空间移交按原生两步操作：先将接收者升为 Owner，确认成功后再降级原 Owner。中断时可保留两位 Owner；重试先读取成员，最后一个 Owner 不允许移除/降级。平台管理员身份不跟随移交。

## 4. 迁移与验证

迁移 13 保存安装进度，迁移 14 保存员工开户完成记录。空表支持回退再升级；已有安装/开户记录时拒绝破坏性回退。不要删除完成记录来“恢复”成员，也不要自动降级到旧登录模式。

```sh
make r2-api-probe
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS='-mod=readonly -count=1' make phase1-gateway-test
# 使用 Node 24
make product-test-frontend PRODUCT_FRONTEND_TEST_ARGS=--offline
make r2-check
```

接口探针使用缓存镜像、随机项目、内部网络、无公开端口和可丢弃 PostgreSQL；`finally` 只清理自己的容器/卷与合成密钥。模型不调用真实服务，企业 OAuth 使用合成 provider。桌面浏览器截图使用真实 Vue 页面与模拟 API，不能替代公司 IdP、TLS、生产模型和实际主机的部署验收。
