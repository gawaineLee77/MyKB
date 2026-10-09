# 单条回答 Word 导出验收

实施日期：2026-10-08；验收更新：2026-10-09。范围为主站普通问答及智能体最终回答的单条 `.docx` 导出；保留本轮之前的图谱浏览工作及 R1–R5 历史验收。本轮不部署企业服务、不修改上游源码、不更改后端权限。

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
