# 第三方依赖与来源

新自有源码使用Apache-2.0；以下依赖各自的许可继续有效。精确版本及完整传递依赖以[go.mod](go.mod)、[go.sum](go.sum)和[package-lock.json](web/package-lock.json)为准。

| 直接依赖 | 用途 | 上游许可与来源 |
|---|---|---|
| coreos/go-oidc | 标准OIDC发现与令牌验证 | [Apache-2.0](https://github.com/coreos/go-oidc/blob/v3.21.0/LICENSE) |
| go-jose/v4 | 仅测试发行者的JWT签名 | [Apache-2.0](https://github.com/go-jose/go-jose/blob/v4.1.4/LICENSE) |
| go.yaml.in/yaml/v3 | 严格YAML解析与渲染 | [Apache-2.0及MIT，按文件适用](https://github.com/yaml/go-yaml/blob/v3.0.5/LICENSE) |
| golang.org/x/oauth2、x/sys | OAuth2交换及单实例OS文件锁 | [BSD-3-Clause](https://go.googlesource.com/oauth2/+/refs/tags/v0.37.0/LICENSE)、[BSD-3-Clause](https://go.googlesource.com/sys/+/refs/tags/v0.49.0/LICENSE) |
| modernc.org/sqlite | 无CGO的本地SQLite | [BSD-3-Clause](https://gitlab.com/cznic/sqlite/-/blob/v1.61.0/LICENSE)；其生成源码和传递依赖亦需保留各自声明 |
| React、React DOM | 管理界面 | [MIT](https://github.com/facebook/react/blob/v19.3.1/LICENSE) |
| lucide-react | 内嵌界面图标 | [ISC](https://github.com/lucide-icons/lucide/blob/main/LICENSE)；具体版本固定于lock |
| TypeScript、Vite、React Vite插件、类型定义 | 构建工具，运行时不需要Node | 各包自带LICENSE；精确分发集合以package-lock和构建产物为准 |

## 独立Mihomo校验器

Mihomo不被Go链接，也不包含在当前公开源码或正式发布包中。本地工具从上游下载指定平台的官方归档，同时验证归档和解包程序的SHA256；运行时再次检查程序摘要。

- [锁定版本、源码commit和工件摘要](internal/validator/lock.json)。
- [上游GPLv3许可](https://github.com/MetaCubeX/mihomo/blob/3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8/LICENSE)。
- [对应源码](https://github.com/MetaCubeX/mihomo/tree/3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8)。

独立进程不免除再分发义务。正式打包任何依赖或校验器之前，必须将适用许可、版权声明和相应源码义务落实到该工件；本文件不替代完整分发声明。当前没有包含Mihomo的Subweft正式二进制或容器发行。

## MaxMind 测试样例

原生 GEOIP 测试按 tools/fetch_testdata.py 固定 MaxMind-DB commit 和 SHA256，获取 Apache-2.0 的 GeoIP2-Country-Test.mmdb。来源：https://github.com/maxmind/MaxMind-DB/tree/d1551916cff0712d12ff602c309234471b0402a2 。这是非生产样例，不随应用工件分发，不代表完整地理数据覆盖。

## Container syscall policy

`deploy/seccomp.json` derives from Moby profiles (Apache-2.0), commit `6fe7deb1b9fb7c0397a4593480d7d22b9ee8caef`, `seccomp/default.json` ([upstream](https://github.com/moby/profiles/blob/6fe7deb1b9fb7c0397a4593480d7d22b9ee8caef/seccomp/default.json)). It retains the default deny behavior and adds the documented nested-namespace operations for bubblewrap; see [container installation](docs/container.md). No application image is published with this candidate.
