# 使用手册导出

源文档为 [MindCreek 使用手册](../../docs/guides/MINDCREEK_USER_MANUAL_ZH.md)。
示例脚本和合成练习资料位于 `docs/examples/user-manual`。

使用 Node 24 和已安装的前端依赖执行：

```sh
node tools/docs/export-user-manual.mjs
```

默认输出到被 Git 忽略的 `images/archives/mindcreek-user-manual-20261010`。
也可传入独立输出目录。

导出器将已有截图和示例下载内容嵌入 HTML。
正文阅读不需要服务器或网络。
扩展参考指向文档声明的产品版本和对应上游提交。
导出不会修改原截图、上游源码或部署包。

发布前检查文档链接、Bash/JSON 示例语法、浏览器目录、图片、下载文件和窄屏排版。
示例请求只能在隔离合成环境验证，不能把脚本检查标记为企业 API 验收。
