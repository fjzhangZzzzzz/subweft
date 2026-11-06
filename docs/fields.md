# 固定版本字段契约

权威目录为 [catalog.json](../internal/fields/catalog.json)，绑定 Mihomo v1.19.33、commit 和源码归档 SHA256。后台与界面读取同一目录；[提取工具](../tools/catalog/main.go)和[审阅覆盖](../tools/catalog/review.go)是更新入口。嵌套对象、数组和动态映射均为结构化表单。

| 协议 | 顶层字段数（含 type） |
|---|---:|
| anytls | 29 |
| http | 20 |
| hysteria | 33 |
| hysteria2 | 36 |
| snell | 16 |
| socks5 | 19 |
| ss | 18 |
| ssr | 17 |
| trojan | 29 |
| tuic | 39 |
| vless | 38 |
| vmess | 41 |
| wireguard | 27 |

目录同时包含 7 类 SS 插件、4 类组的高级字段、DNS 及受支持客户端全局设置。类型为文本、整数、数值、布尔、数组、对象、映射及明确联合值；DNS policy 使用有序条目。required、enum、范围及 secret 标记参与检查和表单生成。

## 产品约束

- 未知类型/键、错误标量、空值及重复字段明确拒绝，发布仍执行固定原生校验。
- 凭据按协议隔离，切换协议清除旧参数。列表不含凭据，显式编辑读取完整记录；预览递归遮蔽密钥、认证、插件及传输敏感字段。
- 证书与 TLS 私钥使用内联 PEM 或文件上传；WireGuard 使用协议要求的 Base64。禁止服务主机文件路径。
- provider/规则 URL、缓存路径及发布清单归服务管理，客户端全局表单不能覆盖。客户端 provider 的代理、健康检查和间隔使用独立表单。
- TLS/传输、Reality、依赖循环及适用字段还需组合检查。字段覆盖和代表样例不等于所有组合、连接或客户端已验收。

## 来源格式

Provider YAML 支持上述 13 协议。完整配置节点提取明确选择后只提取 proxies。URI 当前支持 SS、VMess v2 JSON、VLESS、Trojan、HTTP(S)、SOCKS5 的明确参数及单层 Base64；其他 URI 形式明确拒绝，可使用协议表单或 provider YAML。

规则资源支持 domain/ipcidr/classical 的 YAML payload 和逐行 text；MRS 不在当前格式清单。MMDB/GeoIP/GeoSite 作为受管理二进制资源随发布固定摘要。真实连接和客户端边界见[状态](status.md)。
