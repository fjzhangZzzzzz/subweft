# 本地运行

完整操作路径见[人工验收入口](human-acceptance.md)。本地工具仅用于独立开发/验收，不修改生产配置。真实登录、客户端使用及生产切换状态见[能力状态](status.md)。

## 启动

需要已有 Git、Go 1.27、Node.js（含 npm）和 Python 3.10+。本机开发使用 Go 1.27.3；CI 按 go.mod 使用 Go 1.27.0，Node 固定 26.12.1。工具不安装或升级系统软件。

```sh
python tools/dev.py
```

打开 `http://127.0.0.1:18880`，以测试身份继续。独立测试 OIDC 与来源位于 `http://127.0.0.1:18881`。`/provider.yaml` 提供两条 SS 节点，`/all.yaml` 包含 13 协议代表节点，`/migration.yaml` 提供原模板语义的完整迁移样例；全部为虚构凭据和文档 IP，没有实际代理连接。

工具校验并取得锁定 Mihomo 和测试地理数据，按 package-lock 安装依赖，构建内嵌 UI、静态 Go 应用及测试发行者，初始化独立加密状态后仅在环回 IP 监听。测试发行者不链接到产品程序。

Ctrl+C 停止两个子进程。状态位于 `output/dev/state`，独立密钥位于 `output/dev/master.key`；再次运行保留数据，缺少其中一个时拒绝自动重建。已构建可使用 `python tools/dev.py --skip-build`。下载需代理可显式添加 `--proxy`；Go/npm 依赖沿用自己的环境，不修改系统代理。

## 手工构建与检查

```sh
npm ci --prefix web --ignore-scripts --no-audit --no-fund
npm run format:check --prefix web
npm run build --prefix web
python tools/fetch_validator.py
python tools/fetch_testdata.py
go test -tags webui ./...
go vet ./...
go mod verify
```

构建服务前设置 `CGO_ENABLED=0`，然后执行 `go build -tags webui -o output/bin/subweft ./cmd/subweft`；Windows 输出使用 `.exe`。Linux 的私有校验启动器必须是静态程序。未使用 webui 标签的服务拒绝启动；Go 构建前必须先生成前端。

`subweft version` 输出版本、Git commit、dirty 状态及平台。候选包由[打包工具](../tools/package.py)从干净已提交源码重建前端和三个平台程序，携带 BUILD.json、依赖许可和 SHA256SUMS；不携带 Mihomo、测试发行者、私有数据或密钥。候选工件与正式发布分别记录。

## 运行参数

初始化显式执行 `subweft init --data-dir <目录> --key-file <独立密钥文件>`。之后 `serve` 不创建丢失的数据库或密钥。

| 环境键 | 含义 |
|---|---|
| SUBWEFT_BASE_URL | 明确的应用 HTTPS 基址，无路径 |
| SUBWEFT_LISTEN | 监听地址，默认 127.0.0.1:8080 |
| SUBWEFT_OIDC_ISSUER | 发现文档中的精确 issuer，默认要求 HTTPS |
| SUBWEFT_OIDC_CLIENT_ID | OIDC client ID |
| SUBWEFT_OIDC_CLIENT_SECRET_FILE | 独立受限 secret 文件，与直接 secret 环境值互斥 |
| SUBWEFT_ADMIN_SUBJECTS | 引导管理员精确 sub，逗号分隔 |
| SUBWEFT_MIHOMO_PATH | 锁定摘要相符的程序绝对路径 |
| SUBWEFT_VALIDATION_DIR | 生产必须为 tmpfs 内的校验目录 |
| SUBWEFT_FETCH_ALLOW_CIDRS | 可选的精确来源目标例外，默认拒绝私网 |
| SUBWEFT_LOCAL_HTTP | 仅用于明确环回 IP 的 HTTP 应用和监听；issuer 可为环回 HTTP 或真实 HTTPS |

生产安装、Secret 挂载、Nginx、升级与恢复使用[容器说明](container.md)。认证发现采用延迟连接，IdP 离线不阻止服务重启及已有设备下载；登录仍失败关闭。校验器或命名空间不可用时不降级发布。

## Linux 开发隔离与完整性能复现

Linux 需发行版 bubblewrap、util-linux/prlimit，以及允许 bwrap 建立非特权 user/file/network namespace 的 AppArmor profile。Ubuntu 24.04 可核对发行版 apparmor-profiles 中的 bwrap-userns-restrict；CI 只在临时 runner 加载它。参考[上游 profile](https://gitlab.com/apparmor/apparmor/-/blob/master/profiles/apparmor/profiles/extras/bwrap-userns-restrict)。不要全局关闭 userns 限制或共享宿主网络。Windows 仅验收开发功能，不具备 Linux 命名空间隔离。

构建应用与 devfixture 后执行：

```sh
python tools/benchmark.py
```

工具独立启动真实应用进程，通过实际 OIDC/HTTP API 保存 10 个远程来源定义和 30 万条本地规则（上游合成数据含 2000 节点，服务端不获取），发布 5 个方案、登记 20 台设备；测量 30 秒稳定后的应用 RSS、200 次/10 并发缓存配置下载以及版本化资源，最后下载一致性备份，用实际 CLI 恢复到新目录并重启，对比全部设备配置。只输出汇总，不输出 token 或配置正文，清理自己的合成数据和进程。

Linux 使用 wait4 分别记录 Mihomo 子进程和整个校验任务（含启动器）的操作系统高水位 RSS；Windows 该项明确未测。容器测试另记录 cgroup 总峰值，包含应用、测试驱动、测试发行者和子进程。外网延迟及实际代理连接不在该性能数字内。
