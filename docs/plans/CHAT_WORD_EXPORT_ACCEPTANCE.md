# 单条回答 Word 导出验收

实施日期：2026-10-08；验收更新：2026-10-22。范围为主站普通问答及智能体最终回答的单条 `.docx` 导出；保留本轮之前的图谱浏览工作及 R1–R5 历史验收。本轮不部署企业服务、不修改上游源码、不更改后端权限。

## 2026-10-22 带标注箭头源码修复

旧导出器将 KaTeX 为 `\xrightarrow` 标注生成的 `mpadded` 当作不支持的结构，导致第十个公式所在的整条回答无法导出。本次只在产品 `word-math.ts` 中识别三种单线箭头（右、左、双向）的固定水平标签留白，生成 Word 原生 `groupChr` 和必要的 `limLow`；保留公式结构、上下标注及方向。不接受任意位移／遮盖节点，不自动纠正未知 LaTeX 命令。

| 检查 | 结果 |
|---|---|
| 完整前端 | 最终源码 646 项测试、类型检查和生产构建通过 |
| `make stage1-check` | 产品覆盖层及上游边界通过，上游未修改 |
| 浏览器真实转换函数 | 修复前复现第 10 个公式 `mpadded` 错误；修复后 10 个可编辑公式和 1 个表格成功导出 |
| 新增公式回归 | 17 种方向／上下标注／空标注／复杂标注／嵌套组合，以及行内、独立公式和表格通过；9 种不支持排版或未知命令明确拒绝。共 276 条检查（包括 ZIP 各 XML 文件有效性），无外联、图片降级或页面脚本错误 |
| 浏览器前后截图 | 已检查；是调用真实导出函数的验证页，不是 Word 窗口截图 |
| DOCX 渲染 | 已用 bundled LibreOffice 实际渲染两份样例并检查全部两页；发现其 `groupChr` 导入不兼容，不能据此判定 Word 视觉验收通过 |
| 原生 Word 显示 | 用户解锁后，已打开主样例并核对第十个公式：两条箭头、完整中英标注、γ／λ 显示正确，无修复提示；[实际 Word 截图](../assets/word-arrow-20261022/word-native-tenth-formula.png)。额外双标签样例仅完成结构验证，不冒充 Word 窗口实测 |
| 本机编辑保存 | 用户明确本轮不要求尝试本机 Word 修改，以导出内容正确、界面无报错为准。按此范围收尾；未将编辑保存记为通过，也未更改 Office 激活或账户状态 |

兼容性检查已确认：本机 Word 官方 `omml2mathml.xsl` 可从生成文件恢复箭头与上下标签；这是结构验证，不代替 Word 实际打开、编辑、保存。LibreOffice 26.8 开发版的导入器忽略 `vertJc`，把标签错误放在基线、箭头变成下标；双标注形成重复下标而报错。见[精确版本导入源码](https://github.com/LibreOffice/core/blob/2c87e51eeaa2b413ff4ae097b2705eea1995d8e5/starmath/source/ooxmlimport.cxx)。本次保留 Word 原生结构。

`groupChr` 基线和标注字号依据 [Microsoft 分组字符属性](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.math.groupcharproperties?view=openxml-3.0.1)、[参数字号](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.math.argumentsize?view=openxml-3.0.1)及本机 Word 自带转换器核对。Word 不精确复制 KaTeX 的留白和最小长度；双标注中下方更长时仍可能超过箭头宽度。

可重复执行入口：`tools/word-export/check-arrow-browser.mjs`，样例：`testdata/word-export/annotated-arrows.md`。需准备 `.local/word-arrow-20261022/ui` 隔离前端、依赖及 `before-word-math.ts` 基线；`WORD_EXPORT_BUILD` 可指定其他目录。[本轮证据](evidence/word-arrow-export-20261022.json)记录源码、产物指纹和验证边界。本次已完成用户要求的功能修复与验证，仅交付源码，未封包、构建镜像或部署；不是新的完整发布验收。

## 实现

| 模块 | 行为 |
|---|---|
| `tools/frontend-overlay/apply-word-export.mjs` | 对隔离前端副本增加两个回答入口；明确 JSZip 直接依赖及固定版本 |
| `WordExportButton.vue` | 回答完成后显示；按需加载、下载、防重复点击和转换失败提示 |
| `word-export.ts` | Markdown/简单 HTML 表格生成 Word 原生文本、表格、引用、链接、编号和 DOCX 包 |
| `word-math.ts` | KaTeX MathML 到受支持 OMML 结构；不支持即失败，禁止外部资源和图片降级 |

前端复用回答已有数据，不发出新的业务 API 请求。四角色均可导出各自已显示回答；权限规则、平台导航、知识检索和企业身份不变。图片只保留说明并提示，复杂 HTML 表格不静默降级。

## 验证

使用 Node 24、真实 Chrome 和合成内容，未调用企业 OAuth、真实模型或私有文档。验证脚本在隔离的 `.local/word-export/ui` 前端副本执行；本轮不以历史测试代替新增功能检查。

| 检查 | 结果 |
|---|---|
| 前端单元测试 | 608 项通过，包含 6 项 Word 导出回归 |
| 类型检查与生产构建 | 通过；保留现有大 chunk 提示 |
| 浏览器公式/文件结构 | 131 项断言通过；样例 1 个表格、12 个单元格、11 个 OMML 公式；额外覆盖矩阵、分段、上下标、极限等 |
| 浏览器产品界面 | 四角色下载、智能体回答下载、错误提示且无下载、导出无业务 API 请求通过 |
| 文件渲染 | LibreOffice 生成的两页逐页检查；渲染环境以本机中文字体补充字体配置，不嵌入或分发字体 |
| 原生 Word 打开及编辑保存 | 本机 Word 打开合成样例，表格及公式显示正常，无修复提示。编辑保存未验证：Word 明示未激活、仅查看；未尝试激活或更改账户。企业终端仍须实际验收 |
| 设计、链接及上游边界 | `phase0-check`、`stage1-check` 通过；上游源码保持干净 |

执行入口：

```sh
# 在产品隔离前端副本中
node --import tsx --test
npm run type-check
npm run build

# 仓库根目录；Playwright 由验证环境提供
node tools/word-export/check-browser.mjs
node tools/word-export/check-ui.mjs
make phase0-check stage1-check
```

两个浏览器脚本仅监听本机。`check-ui.mjs` 默认使用图谱实现完成时保留的 `.local/graph-view/ui/dist` 作为变更前截图基线，也可通过 `WORD_EXPORT_BEFORE_DIST` 指定先前构建。重跑时需准备隔离前端及依赖，不在上游工作树应用 overlay。

结构化结果、源码与日志指纹见[本轮证据](evidence/word-export-verification.json)。可下载[合成样例](../assets/word-export/editable-answer.docx)进行企业终端编辑保存验收。样例的 LibreOffice 渲染已逐页检查；本机 Word 打开检查使用补充现代兼容模式标记之前的等内容测试副本，编辑保存受本机 Office 激活状态限制，未记为通过。

## 界面证据

- [变更前](../assets/word-export/before-chat.png)
- [普通回答导出](../assets/word-export/after-chat.png)
- [智能体回答导出](../assets/word-export/after-agent.png)
- [不支持的公式](../assets/word-export/unsupported-formula.png)

## 交付边界

使用与限制见[指南](../guides/CHAT_WORD_EXPORT_ZH.md)。未实现整段会话导出、小助手导出按钮、图片嵌入、截图表格 OCR 或任意 LaTeX 转换。企业目标 Word/浏览器、前端镜像更新和实际部署验收不由本机合成测试替代。
