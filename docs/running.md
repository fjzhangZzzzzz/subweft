# 本地运行

完整操作路径见[人工验收入口](human-acceptance.md)。本地工具仅用于独立开发/验收，不修改生产配置。真实登录、客户端使用及生产切换状态见[能力状态](status.md)。

## 启动

需要已有 Git、Go 1.27、Node.js（含 npm）和 Python 3.10+。本机开发使用 Go 1.27.3；CI 按 go.mod 使用 Go 1.27.0，Node 固定 26.12.1。工具不安装或升级系统软件。

```sh
python tools/dev.py
```

打开 `http://127.0.0.1:18884`，以测试身份继续。独立测试 OIDC 位于 `http://127.0.0.1:18885`。测试身份仅用于本机验收。示例导入文件见 `docs/examples/editor.yaml`；使用保留域名，不提供实际代理连接。

工具校验并取得锁定 Mihomo 和测试地理数据，按 package-lock 安装依赖，构建内嵌 UI、静态 Go 应用及测试发行者，初始化独立加密状态后仅在环回 IP 监听。测试发行者不链接到产品程序。测试 Country.mmdb 仅能用于校验流程，不能当作真实地理数据库。新编辑器使用独立数据目录，旧数据不迁移。

Ctrl+C 停止两个子进程。状态位于 `output/editor-acceptance/state`，独立密钥位于 `output/editor-acceptance/master.key`；再次运行保留数据，缺少其中一个时拒绝自动重建。已构建可使用 `python tools/dev.py --skip-build`。下载需代理可显式添加 `--proxy`；Go/npm 依赖沿用自己的环境，不修改系统代理。

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
| SUBWEFT_ADMIN_SUBJECTS | 唯一管理员精确 sub，不接受逗号分隔的多个身份 |
| SUBWEFT_MIHOMO_PATH | 锁定摘要相符的程序绝对路径 |
| SUBWEFT_VALIDATION_DIR | 生产必须为 tmpfs 内的校验目录 |
| SUBWEFT_GEODB_DIR | 部署预备的 GeoDB 目录；GEOIP 检查缺少所需文件会失败，不自动下载 |
| SUBWEFT_LOCAL_HTTP | 仅用于明确环回 IP 的 HTTP 应用和监听；issuer 可为环回 HTTP 或真实 HTTPS |

生产基础设施边界参考[容器说明](container.md)，本版切换步骤以[迁移与切换](editor-cutover.md)为准。认证发现采用延迟连接，IdP 离线不阻止服务重启及已有设备下载；登录仍失败关闭。校验器或命名空间不可用时不降级发布。

## 验证边界

Windows 仅用于开发功能验收，不具备 Linux 命名空间隔离。Linux 生产仍须按容器说明配置 bubblewrap 等限制，并重新执行新服务的部署验收。

`tools/benchmark.py`、`windows_check.py` 和 `container_check.py` 中旧业务 API 场景尚未迁移到新模型，不作为本版验收入口，也不能沿用旧性能数字。当前可重复检查使用完整 Go 测试、前端构建、锁定 Mihomo 测试及 human-acceptance.md 的浏览器流程。
