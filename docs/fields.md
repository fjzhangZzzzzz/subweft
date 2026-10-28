# 已实现闭环的字段矩阵

此矩阵描述当前代码接受的字段，不取代[完整首版目标](support-matrix.md)。本轮固定Mihomo v1.19.33，三平台归档与解压后程序摘要见 [校验器锁](../internal/validator/lock.json)。节点支持ss、vmess、vless、trojan、http、socks5的下述明确字段子集；未列字段和协议明确拒绝，不隐藏透传。

## 来源

| 字段 | 类型与默认 | 校验和语义 |
|---|---|---|
| name | string，必填 | 表单去首尾空白，API拒绝首尾空白；名称前缀用于区分来源 |
| kind | `http` / `local` | HTTP抓取，或显式内容导入 |
| url | string | 仅HTTP来源；HTTP/HTTPS、禁止userinfo和fragment；编辑留空保留 |
| content | string | 仅本地导入；provider YAML、下述代理URI列表或单层Base64封装；编辑留空保留 |
| enabled | bool，界面默认true | 停用保留数据，不能作为显式选中来源发布 |
| tags | string[] | 动态来源标签，最多32个 |
| interval | integer，3600秒 | 60至604800；后台更新有效快照，不自动发布草稿 |
| id / revision | 服务生成 | 定义编辑采用乐观并发检查；后台缓存刷新不改变定义修订号 |

provider输入根仅允许 `proxies`。拒绝多文档、别名、未知字段、错误标量类型、重复节点名、空结果、超过5000节点或2MiB输入。支持单层Base64封装的provider或URI列表；完整配置提取及其他导入格式仍在后续范围。

服务端抓取执行每连接/重定向地址校验、DNS解析结果固定拨号、超时和体积限制。默认拒绝私网、环回等目标，运维可设置明确CIDR允许列表。抓取代理及其表单尚未实现。

## Shadowsocks节点

| Mihomo字段 | 类型 | 当前校验 / 默认 |
|---|---|---|
| name | string | 必填、禁止逗号及换行，输出时加来源前缀 |
| type | string | 固定 `ss` |
| server | string | 必填、长度与控制字符检查；由原生校验进一步检查 |
| port | integer | 1至65535；禁止浮点、字符串隐式转换 |
| cipher | string | `aes-128-gcm`、`aes-256-gcm`、`chacha20-ietf-poly1305` |
| password | string | 必填，最长2048；手工节点编辑留空保留 |
| udp | bool | YAML省略为false；新建手工节点表单默认true |

手工节点另有enabled、id和revision。SS插件、2022加密方式和其他通用高级字段尚未实现；不能将当前SS子集标为完整SS覆盖。来自订阅的协议字段跟随上游；可复制为手工节点后独立修改。来源节点本地显示/排除覆盖仍待实现。

## VMess、VLESS、Trojan、HTTP及SOCKS5

固定版本字段依据：[VMess](https://github.com/MetaCubeX/mihomo/blob/3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8/adapter/outbound/vmess.go)、[VLESS](https://github.com/MetaCubeX/mihomo/blob/3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8/adapter/outbound/vless.go)、[Trojan](https://github.com/MetaCubeX/mihomo/blob/3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8/adapter/outbound/trojan.go)、[HTTP](https://github.com/MetaCubeX/mihomo/blob/3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8/adapter/outbound/http.go)、[SOCKS5](https://github.com/MetaCubeX/mihomo/blob/3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8/adapter/outbound/socks5.go)。

下表字段均有结构化表单、严格YAML/API类型检查和渲染。原生样本分别覆盖，实际代理连接和客户端验收仍未完成。

| 字段 | 适用协议 | 当前语义 |
|---|---|---|
| name / type / server / port | 全部 | 与上述基础字段相同；type必须在六种已支持协议中 |
| uuid | vmess、vless | 标准UUID，作为可恢复凭据加密保存 |
| cipher | vmess | auto、aes-128-gcm、chacha20-poly1305、none、zero |
| alterId | vmess | 0至65535；默认0，渲染仍显式输出0 |
| password | trojan | 必填，最长2048 |
| username / password | http、socks5 | 成对填写或同时清除；新账号不能隐式沿用旧密码 |
| udp | vmess、vless、trojan、socks5 | bool；HTTP没有此字段 |
| tls | vmess、vless、http、socks5 | bool；Trojan始终TLS，不接收额外tls字段 |
| skip-cert-verify | 除ss以外 | 默认false；必须启用TLS，界面明确标识不安全 |
| servername | vmess、vless | TLS服务器名称；不将其他协议的SNI字段静默转换 |
| sni | trojan、http | TLS服务器名称；SOCKS5的该字段不在当前锁定契约中 |
| alpn | vmess、vless、trojan | 最多8个非空字符串；表单区分内核默认、自定义列表及显式空列表；存储与渲染保留该区别 |
| client-fingerprint | vmess、vless、trojan | chrome、firefox、safari、ios、android、edge、random、randomized；或省略 |
| network | vmess、vless、trojan | TCP（省略或tcp）、ws、grpc；其他传输明确拒绝 |
| ws-opts | vmess、vless、trojan的ws | path、headers、max-early-data（0至65536）、early-data-header-name；标头为最多32个字符串键值对，禁止换行及忽略大小写的重名 |
| grpc-opts | vmess、vless、trojan的grpc | grpc-service-name；其他gRPC字段待覆盖 |
| flow | vless | 空或xtls-rprx-vision；Vision要求TLS和TCP |
| reality-opts | vless | public-key为32字节Raw Base64URL，short-id为0至8字节十六进制；要求TLS、servername、客户端指纹且验证证书 |

只接受所选协议和传输适用的字段；例如SS的tls:false、HTTP的uuid及TCP的ws-opts也会拒绝。嵌套选项同样拒绝未知字段、错误类型和null。SS插件/2022、更多传输、Reality在其他协议中的应用、证书文件、额外HTTP/SOCKS字段和其他高级参数仍未覆盖。

### 导入格式

- provider YAML根只含proxies；逐节点按协议检查，不跳过不支持的节点。
- ss://采用SIP002用户信息格式，暂不接受插件参数或整段地址的旧式Base64编码。
- vmess://采用Base64 JSON，支持v、ps、add、port、id、aid、scy、net、type（空/none）、host、path、tls、sni、alpn、fp。该URI格式的port/aid明确接受整数或数字字符串，不放宽YAML/API类型。未知键拒绝。
- vless://及trojan://按已支持的TCP/WS/gRPC与TLS参数解析；VLESS还支持encryption=none、flow、Reality的pbk/sid。未知、重复或与传输不符的查询参数拒绝，不静默忽略。
- http://、https://、socks5://代理URI支持可选成对账号密码；没有HTTP path/query扩展。HTTPS对应http加tls。它们是节点内容，不是订阅抓取地址。
- 上述URI可混合逐行输入，或包裹一层标准/Base64URL编码。尺寸及节点数限制不变，不递归解码。

### 列表、编辑与脱敏

管理列表和保存响应仅返回节点名称、协议、服务器、端口及UDP摘要；UUID、账号密码、Reality和传输参数不会预取到列表。管理员明确点击编辑才通过受会话保护、no-store的GET /api/v1/nodes/<id>/edit读取完整参数。凭据输入默认遮蔽，可主动显示；同协议编辑留空保留密码/UUID，更换协议不继承凭据。HTTP/SOCKS5关闭账号密码可清除认证。

预览遮蔽password、username、uuid、Reality short-id、WS标头值/路径及gRPC服务名；实际发布保留完整值。数据库仍对完整记录加密，未改变主密钥和既有设备凭据。旧SS数据库无需重建，运行升级与完整恢复验收分开记录。

## 策略组

| 字段 | 类型与默认 | 说明 |
|---|---|---|
| name | string | 方案内唯一，禁止与节点或内置出口冲突 |
| type | enum | select、url-test、fallback、load-balance |
| sourceIds / tags / allSources | string[] / string[] / bool | 显式来源、标签和全部启用来源取并集 |
| nodeIds / groups / builtins | string[] | 手工节点、其他组、显式DIRECT/REJECT，按出现顺序去重 |
| filter / exclude | string | Go正则，最长512；用于节点，不过滤显式组/内置出口 |
| url | string | 非select的客户端健康检查URL，默认公共204检查地址 |
| interval | integer，300 | 非select，30至86400秒 |
| tolerance | integer，50 | url-test，0至10000毫秒 |
| strategy | enum | load-balance：consistent-hashing、round-robin、sticky-sessions |

表单重命名组同时更新当前草稿中的组及规则引用。发布拒绝循环、缺失引用和空组，不静默改为直连。节点扁平化输出为自包含配置；此闭环未输出proxy-provider资源路由，原provider客户端字段的完整迁移仍待完成。

## 规则和客户端设置

规则有type、value、target、noResolve、enabled；已实现DOMAIN、DOMAIN-SUFFIX、DOMAIN-KEYWORD、IP-CIDR、IP-CIDR6、MATCH。目标为方案组或DIRECT/REJECT；no-resolve仅用于IP规则；必须恰有位于末尾的启用MATCH。支持表单添加、编辑、删除、启停和上下移。规则集、GEOIP和复杂逻辑条件尚未实现。

客户端设置：mixedPort（1至65535，默认7890）、allowLan（false）、ipv6（false）、dnsEnabled（false）、dnsMode（fake-ip/redir-host）和nameservers（字符串列表）。DNS其他字段、TUN及更多全局设置不在当前闭环，不能据此宣布已完整迁移旧配置。

## 设备、发布与认证

设备字段为name、profileId、state（active/paused/revoked）、可选expiresAt和revision。必须绑定已有发布的方案。Token由服务生成，加密保存并以HMAC索引；列表不返回Token，管理员主动查看链接才返回。吊销不能恢复，轮换立即废止旧Token。普通发布与方案切换保持URL稳定。

发布记录完整配置、来源摘要、方案修订、校验器身份和创建时间，同一SQLite事务写入正文与活动指针；每方案最多保留最新20份且不删除活动版本。回滚只切换不可变发布，草稿不变。配置编译和校验期间若来源或定义改变，则拒绝过期候选。

OIDC实现issuer/sub管理员准入、PKCE、nonce、单次state、8小时会话、CSRF和退出撤销。仅管理员角色已实现；普通使用者及方案授权仍待扩展。真实Pocket ID和Passkey未验收，本地测试使用独立RS256测试发行者，不绕过产品认证。
