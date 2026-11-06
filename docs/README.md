# 文档导航

当前正式版 v0.4.1 在 v0.3.1 基础上将固定订阅池调整为可扩展标签，并提供一次性迁移和集中标签管理。本次发布不包含部署操作。

- [菜单交互验证](menu-interaction-review.md)：菜单关闭、键盘焦点及手机导航。
- [登录页视觉评审](login-visual-review.md)：黑白共享样式、响应式登录与验证边界。
- [性能报告](performance-report.md)：三档规模、真实发布、并发下载与浏览器加载。
- [人工验收](human-acceptance.md)：入口、操作步骤与实际验证边界。
- [本地运行](running.md)：独立本地状态和测试身份。
- [GeoDB 校验边界](adr/0009-geographic-validation-boundary.md)：部分原生检查、可选数据库及发布承诺。
- [DNS 结构化编辑](adr/0010-dns-structured-editor.md)：基础配置入口、字段范围与编辑边界。
- [产品设计](design.md)、[架构](architecture.md)、[支持矩阵](support-matrix.md)：正式范围和实现边界。
- [验收标准](acceptance.md)、[当前状态](status.md)、[实施清单](implementation-reset.md)。
- [新方向决策](adr/0008-config-editor-reset.md)、[已通过原型](prototype-review.md)。
- [迁移与切换](editor-cutover.md)：保持旧服务，先验证再切换。

`fields.md`、旧 ADR、旧容器验收及旧工作区源文件保留追溯。当前正式入口是 `internal/server/editor.go` 和 `web/src/editor.js`；不得将旧业务模块的功能宣传为本版支持。
