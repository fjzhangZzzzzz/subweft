# 已实现闭环的字段矩阵

此矩阵描述当前代码接受的字段，不取代[完整首版目标](support-matrix.md)。本轮固定Mihomo v1.19.33，三平台归档与解压后程序摘要见 [校验器锁](../internal/validator/lock.json)。节点仅支持下表的SS子集；未列字段明确拒绝，不隐藏透传。

## 来源

| 字段 | 类型与默认 | 校验和语义 |
|---|---|---|
| name | string，必填 | 去首尾空白；名称前缀用于区分来源 |
| kind | `http` / `local` | HTTP抓取，或显式内容导入 |
| url | string | 仅HTTP来源；HTTP/HTTPS、禁止userinfo和fragment；编辑留空保留 |
| content | string | 仅本地导入；provider YAML或SIP002 `ss://`列表；编辑留空保留 |
| enabled | bool，界面默认true | 停用保留数据，不能作为显式选中来源发布 |
| tags | string[] | 动态来源标签，最多32个 |
| interval | integer，3600秒 | 60至604800；后台更新有效快照，不自动发布草稿 |
| id / revision | 服务生成 | 定义编辑采用乐观并发检查；后台缓存刷新不改变定义修订号 |

provider输入根仅允许 `proxies`。拒绝多文档、别名、未知字段、错误标量类型、重复节点名、空结果、超过5000节点或2MiB输入。完整配置和Base64整包等输入仍在后续范围。

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
