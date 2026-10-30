# Subweft

**面向个人与家庭的 Mihomo 可视化配置编辑器。**

[English](README.en.md) · [产品设计](docs/design.md) · [支持矩阵](docs/support-matrix.md) · [人工验收](docs/human-acceptance.md)

当前候选：**v0.3.0-rc.1**。按已通过评审的单配置模型重建，准备供人工验收；不表示真实客户端或生产切换已经通过。

## 使用方式

订阅 → 策略组 → 规则集 → 分流规则 → 查看差异 → 检查并发布 → 设备更新。

- 唯一配置、唯一部署指定的 OIDC 管理员；无用户角色和多方案管理页面。
- 订阅仅维护 HTTP(S) 地址与客户端参数，分别属于高质量池或高流量池。服务不抓取、解析或缓存上游节点。
- 策略组支持 select / url-test、一个或两个池以及 DIRECT / REJECT；不支持手工节点或组嵌套。
- 自维护规则集按用途组织，混合域名和 IP 网段，自动生成可读的 `用途-domain` / `用途-ipcidr` provider。
- 所有日常编辑通过表单保存草稿；Git 风格脱敏差异、固定 Mihomo 检查及显式发布。历史恢复仅生成草稿。
- 设备独立授权、有效期、暂停、恢复、撤销及链接轮换立即生效。Nginx 只处理 TLS 和反代。

基础 DNS/端口通过首次支持子集导入，随后只读。具体限制见[支持矩阵](docs/support-matrix.md)，不要将旧工作区模块的能力视为新版支持。

## 本机试用

需要 Go、Node.js/npm、Python 和 Git，工具链版本见[本地运行](docs/running.md)。

```sh
python tools/dev.py
```

打开 <http://127.0.0.1:18884/>，使用本机合成 OIDC 身份。程序使用独立开发数据库与密钥，不修改生产。

## 发布与验收边界

提供 Linux amd64、Linux arm64 和 Windows amd64 应用包；Windows 用于开发试用。安装包不携带 Mihomo，校验器按固定摘要单独取得。SQLite 是业务状态权威，密钥独立保存。新模型使用独立数据目录，不能原地升级旧工作区数据库；见[迁移与回退](docs/editor-cutover.md)。

[性能报告](docs/performance-report.md)记录开发基线：普通家庭规模表现良好，大配置和 20 个历史版本下存在管理响应体过大及并发下载排队问题，尚未优化。真实 Pocket ID、真实订阅、Clash Verge 和生产替代仍需人工验收。

源码提交、CI、应用包发布、实际部署和用户验收分别记录。候选下载见 [GitHub Releases](https://github.com/fjzhangZzzzzz/subweft/releases)。

## 许可

自有源码采用 [Apache-2.0](LICENSE)。依赖及独立 Mihomo 校验器保留各自许可，见[第三方声明](THIRD_PARTY.md)。
