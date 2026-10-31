# 有限 DNS 结构化编辑

接受：2026-10-31，用户认可 DNS 编辑方案及双视图预览，授权实施和验证。

调整 ADR-0008 中基础配置整体只读的边界：运行设置仍只读，仅开放明确支持的 DNS 字段。入口在概览标题区，复用基础配置侧栏，不增加导航对象、YAML 编辑器或模板继承。

DNS 常用项包含 enable、ipv6、enhanced-mode、nameserver；高级项包含 respect-rules、default-nameserver、proxy-server-nameserver、nameserver-policy、fallback、fallback-filter 的 geoip/geoip-code/ipcidr/domain、fake-ip-range、fake-ip-filter、use-hosts。布尔值区分未设置、开启和关闭；其他导入字段原样保留且禁止通过此入口修改。

clash-config 预设仅向当前表单填入字段，不保存、不发布、不自动替换已有配置。明确保存后进入现有草稿、差异、检查、发布及恢复流程；DNS 变更使检查失效。服务端执行字段类型、边界与只读字段检查。GeoDB 的检查承诺沿用 ADR-0009，不将缺库解释为所有 DNS 功能已验证。

不更换默认 DNS，不新增外部依赖、解析服务或生产部署操作。真实 DNS 路由、客户端数据和解析效果仍需部署侧验收。
