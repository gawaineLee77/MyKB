# R4：企业登录诊断补丁与 Phase 5 配置核对

版本：`r4-20260916-identity-diag1`。适用于完整 AMD64 包 `r4-20260916-118b8af3`，可直接覆盖此前 `r4-20260916-short-secret-3959a2e2` 网关补丁，保留短 Client Secret 修复。

## 1. 本次解决什么

网关日志现在可以回答：回调是否收到 state/Cookie、为什么返回 invalid_state、授权与回调是否落在同一进程、是否已开始访问企业 Token/UserInfo、哪个阶段遇到 CA/域名/连接错误，以及 `/api/v1/auth/me` 是否被调用者取消。

本补丁沿用实例现有的协议选项和认证判断；不自动修改 state/PKCE、URL、请求方法、用户映射或企业秘密。上游源码和数据库结构保持原状。

记录内容：固定认证路径、方法、状态、耗时、错误分类、非敏感配置、CA 文件指纹与证书数量。日志没有密码、Client Secret、原始 state、Cookie、授权码、Token、请求/响应正文或员工资料。外部端点只记录 scheme/host，不记录可能携带令牌的路径或查询参数。无需将 App 的日志级别调成 info/debug。

## 2. 应用 AMD64 补丁

传输同名 `.tar.gz` 和 `.tar.gz.sha256`，使用原部署账户执行：

```sh
sha256sum -c mindcreek-r4-20260916-identity-diag1-amd64-hotfix.tar.gz.sha256
tar -xzf mindcreek-r4-20260916-identity-diag1-amd64-hotfix.tar.gz
cd mindcreek-r4-20260916-identity-diag1-amd64-hotfix

# 必须沿用实际已有实例的路径及 project。
export MINDCREEK_STATE_DIR=/srv/mindcreek/r4-instance
export MINDCREEK_PROJECT=mindcreek-enterprise-r4
python3 apply.py --package /opt/mindcreek/mindcreek-r4-20260916-118b8af3-amd64
```

工具会验证校验和、基线版本及 AMD64 镜像摘要，并备份/合并 `compose.override.json`。只更新 gateway 和 installer 的镜像选择，原配置其他内容保留；重复应用无重复修改。保留原完整包和八个基线镜像，补丁摘要由 apply.py 单独核对。遇到未知自定义网关镜像会拒绝覆盖。

在短维护窗口内重建网关：

```sh
cd /opt/mindcreek/mindcreek-r4-20260916-118b8af3-amd64
python3 bin/mindcreek validate
python3 bin/mindcreek compose up -d --no-deps --pull never --no-build --force-recreate --wait --wait-timeout 120 gateway
python3 bin/mindcreek compose restart frontend
# 使用包内 TLS 时再执行：
python3 bin/mindcreek compose restart tls
python3 bin/mindcreek compose images gateway
```

本补丁不需要重新 init、建 admin 或建默认空间。重启会清空尚未完成的内存登录事务；重启完成后，用一个新的无痕窗口从主页重新登录一次，避免刷新旧 callback。

## 3. 无需捕捉浏览器瞬间跳转

补丁目录内提供日志提取工具。保留前面的 STATE_DIR/PROJECT，完成一次失败或成功登录后执行：

```sh
# 此处应位于补丁解压目录；绝对路径按实际传输位置调整。
python3 collect.py --package /opt/mindcreek/mindcreek-r4-20260916-118b8af3-amd64 --since 15m > identity-diagnostics.jsonl
```

工具仅提取网关已脱敏的结构化认证事件，不输出任意 App 日志、SQL 或请求正文。可分享这个文件，优先提供 `identity_configuration`、`identity_callback_checked` 以及相邻的 `identity_http_finished`。如果启动早于 15 分钟，可扩大 `--since` 时间窗口。

也可以使用原部署工具查看：

```sh
python3 bin/mindcreek compose logs --since 15m --no-color gateway
```

### 关联字段

| 字段 | 作用 |
|---|---|
| `request_id` | 关联同一次 HTTP 请求及其内部调用 |
| `flow_id` | 关联同一次登录的授权、回调、企业 HTTP 调用及内部 OIDC 交接；随机生成，不是认证凭证 |
| `instance_id` | 本次网关进程标识；重启或另一实例的值不同 |
| `call_id` | 区分同一个 request_id 下多次内部 HTTP 调用 |
| `path` | 固定认证路径，如 `/api/v1/auth/me`；其他资源使用占位路径 |
| `context_state` | `context_canceled` 表示上层上下文取消；`timeout` 表示超时；`none` 表示未取消 |

### invalid_state 的原因

示例只包含布尔值和随机诊断 ID：

```json
{"event":"identity_callback_checked","reason":"state_missing","state_required":true,"state_present":false,"cookie_present":true,"transaction_found":false,"flow_id":"example-flow","instance_id":"example-instance"}
```

| `reason` | 含义与下一步 |
|---|---|
| `state_missing` | 配置要求 state，但回调没有。核对企业是否回传，以及首页中转是否丢失参数 |
| `cookie_missing` | 回调未带登录 Cookie。核对发起/回调域名、HTTPS、浏览器及反向代理 |
| `cookie_mismatch` | state 对应的 Cookie 与当前 Cookie 不一致。检查多窗口重复登录或代理 Cookie 改写 |
| `state_mismatch` | Cookie 可关联当前进程中的登录，但回传 state 未匹配。检查被改写的参数或旧登录页面 |
| `transaction_not_found` | 当前进程找不到记录。检查重启、多网关实例或已消费/清理的旧回调；仅此字段不能区分这些原因 |
| `transaction_expired` | 仍可找到记录，但已超过 10 分钟 |
| `accepted` | 回调状态校验通过；继续查看企业 Token/UserInfo 阶段 |

`state_required=false` 时仍必须具备匹配 Cookie 和有效的一次性登录记录。为日志关联进行的 Cookie 查询不会改变原有认证判断。

### 证书与接口错误

`identity_http_started/finished` 的 `stage` 区分 `corporate_token`、`corporate_userinfo`、OIDC discovery/JWKS 和 `upstream_auth_me`。

| 字段值 | 含义 |
|---|---|
| `tls_unknown_authority` | 对端证书链不受当前网关信任 |
| `tls_hostname_mismatch` | URL 域名与证书 SAN 不匹配 |
| `tls_certificate_time_invalid` | 证书过期或尚未生效；检查证书和系统时间 |
| `dns_failed` / `connection_refused` | 域名解析或服务连接失败 |
| `context_canceled` / `timeout` | 请求取消或超时；结合该调用和外层请求耗时 |
| HTTP 400/401/403/404 | 网络/TLS 已获得 HTTP 响应，继续核对方法、格式、客户端凭证及接口权限 |
| `identity_response_rejected` | HTTP 有响应，但 JSON、Token 字段或 UserInfo 映射不符合配置；`reason` 指明阶段 |

启动事件 `identity_configuration` 记录 `ca_bundle_readable`、`ca_bundle_sha256`、`ca_certificate_count`。这些证明配置的 CA 文件可以读取和解析，不等于每个企业站点均已通过 TLS 验证。`ca.pem` 若是 PEM 格式的企业根/中间 CA，复制为实例 `ca/certificate-ca.crt` 是正确的；更新后需 validate 并重建相关服务。

## 4. 为什么 Phase 5 可以而新企业包失败

本次排查前，企业 OAuth2 适配器没有改动；但新安装模板采用通用 OAuth2 参数，**与原 Phase 5 企业协议配置不同**。源码保留不等于有效请求参数相同。

| 配置项 | 原 Phase 5 文档的企业协议 | 新企业包模板 |
|---|---|---|
| `AUTHORIZATION_METHOD` | POST | GET |
| `TOKEN_REQUEST_FORMAT` | json | form |
| `USERINFO_TOKEN_TRANSPORT` | query | bearer（Compose 默认） |
| `STATE_REQUIRED` | false，Cookie 绑定兼容模式 | true |
| `PKCE_ENABLED` | false | true |
| `SCOPES` | base.profile | openid profile email |
| `SUBJECT_CLAIM` | globalUserID | sub |
| `TENANT_CLAIM / SUBJECT_TENANT_SCOPED` | tenantId / true | tenantId（适配器默认） / false |
| `USERNAME_CLAIM / DISPLAY_NAME_CLAIM` | uid / uid | name / name |
| `EMAIL_CLAIM` | 空，生成内部映射别名 | email |
| 企业登记回调 | 主站根地址，由页面中转 | 必须根据企业实际登记和 REDIRECT_URI 明确设置 |

表中的配置项均带 `MINDCREEK_IDENTITY_` 前缀。如果当前仍使用 Phase 5 的同一身份服务，应核对当时已验证配置，按真实协议恢复相关参数；特别是企业不回传 state 时，保持 `STATE_REQUIRED=true` 会持续得到 invalid_state。兼容模式比提供者返回 state 加 PKCE 的保护弱，适用于已核对的原企业协议，不能作为任意 state 错误的通用解决办法。补丁不会自动切换这些设置。

另有一个确定的前端兼容缺口：原完整包首页 `/?code=...` 中转只保留 code/error，没有保留 state。**如果企业会返回 state，并且回调先落到首页，此中转会丢失 state。** 本网关诊断补丁没有修改前端镜像；提供者支持直接回调时，可将企业登记地址和 `MINDCREEK_IDENTITY_REDIRECT_URI` 一起设为 `/api/v1/mindcreek/oidc/callback` 的完整 HTTPS URL，避开首页中转。原 Phase 5 无 state 模式不受此字段丢失影响。根据实际日志确认路径后再选择适配，不应仅根据 auth/me 的 502 推断原因。

### 4.1 已确认沿用 Phase 5 同一身份服务时

若诊断为 `reason=state_missing`，且仍连接原 Phase 5 企业服务，先恢复该服务已验证的协议配置。当前错误的直接触发项是 `STATE_REQUIRED=true`；下面其余项目用于避免通过回调后继续遇到请求格式或用户字段不匹配。

编辑实际实例的 `$MINDCREEK_STATE_DIR/enterprise.env`，替换已有同名项，缺少的项再添加；不能重复定义，也不要只修改包内 `enterprise.env.example`：

```dotenv
MINDCREEK_IDENTITY_PROTOCOL=oauth2
MINDCREEK_IDENTITY_AUTHORIZATION_METHOD=POST
MINDCREEK_IDENTITY_AUTHORIZATION_GRANT_TYPE=authorization_code
MINDCREEK_IDENTITY_AUTHORIZATION_DISPLAY=page
MINDCREEK_IDENTITY_CLIENT_AUTH_METHOD=client_secret_post
MINDCREEK_IDENTITY_TOKEN_REQUEST_FORMAT=json
MINDCREEK_IDENTITY_STATE_REQUIRED=false
MINDCREEK_IDENTITY_PKCE_ENABLED=false
MINDCREEK_IDENTITY_USERINFO_TOKEN_TRANSPORT=query
MINDCREEK_IDENTITY_SCOPES=base.profile
MINDCREEK_IDENTITY_USERINFO_DATA_PATH=
MINDCREEK_IDENTITY_SUBJECT_CLAIM=globalUserID
MINDCREEK_IDENTITY_TENANT_CLAIM=tenantId
MINDCREEK_IDENTITY_SUBJECT_TENANT_SCOPED=true
MINDCREEK_IDENTITY_USERNAME_CLAIM=uid
MINDCREEK_IDENTITY_DISPLAY_NAME_CLAIM=uid
MINDCREEK_IDENTITY_UUID_CLAIM=uuid
MINDCREEK_IDENTITY_EMPLOYEE_TYPE_CLAIM=employeeType
MINDCREEK_IDENTITY_EMAIL_CLAIM=
```

保留实际企业端点、Client ID/Secret、CA、主站域名以及已有员工准入限制。`MINDCREEK_IDENTITY_REDIRECT_URI` 必须与企业实际登记值完全一致；原登记为 `https://mindcreek.com` 就沿用这个根地址，不要为了本次修复单方面加 callback 路径或斜杠。此处只恢复身份服务协议，不改变 R2/R4 的首次 Viewer 开户和本地 admin 流程。

在原完整包目录执行，沿用部署时的 `MINDCREEK_STATE_DIR` 和 `MINDCREEK_PROJECT`：

```sh
python3 bin/mindcreek validate
python3 bin/mindcreek compose up -d --no-deps --pull never --no-build --force-recreate --wait --wait-timeout 120 gateway
python3 bin/mindcreek compose restart frontend
```

修改环境变量后必须重建网关容器，单独 `restart gateway` 不会更新它的环境。无需更换镜像或重新初始化。网关重建完成后，从新的无痕窗口打开主页登录，不复用之前的 callback。

启动日志应显示 `state_required=false`、`pkce_enabled=false`、`authorization_method=POST`、`token_request_format=json` 和 `userinfo_token_transport=query`。新的回调应能通过 Cookie 找到有效的一次性事务，出现 `reason=accepted`，随后才有 `corporate_token` / `corporate_userinfo` 调用。若转为 TLS 错误，再按具体 `error_kind` 处理证书；通过 state 校验不代表完整企业登录已经成功。

## 5. 验证与回退

包内 `evidence.json` 记录当前源码的无缓存 Go 测试、相关包竞态检查、AMD64 运行的认证/日志测试和补丁重复应用检查。合成 TLS 验证覆盖不信任 CA 与信任 CA 两种结果，Phase 5 的 POST/JSON/query 契约继续验证；诊断测试确认非法回调不会进入 Token 请求，日志不泄漏凭证。

历史 R4 浏览器/API 报告继续作为历史基线，不标记成本补丁已重跑。企业真实登录问题尚需服务器日志确认；BM25/混合检索未通过项仍保持原结论。

回退时，在确认没有后续 override 修改后，恢复 apply.py 打印的 `compose.override.before-hotfix-*.json` 备份，再重建 gateway、重启 frontend。原来没有 override 时，只移除本补丁加入 gateway/installer 的 image/platform/pull_policy 字段，保留其他配置。退回 short-secret 镜像保留短密钥修复；退回最初完整包网关会恢复 16 字符限制。回退不删除业务卷或实例秘密。
