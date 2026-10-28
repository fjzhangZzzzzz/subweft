# 本地运行开发预览

当前可运行六种协议的字段子集的业务闭环，尚未完成完整首版、真实Pocket ID/Clash Verge和生产部署验收。使用虚构来源测试。详情见[字段矩阵](fields.md)和[能力状态](status.md)。

## 一键本地启动

需要已有Git、Go 1.27、Node.js（含npm）和Python 3.10以上。本次开发环境实际使用Go 1.27.3、Node 26.12.1；工具不会安装或升级系统软件。

```sh
python tools/dev.py
```

工具下载并校验锁定Mihomo、按package-lock安装前端依赖、构建内嵌UI和两个程序、初始化独立加密状态，然后仅在环回地址启动：

- 应用：`http://127.0.0.1:18880`
- 测试OIDC发行者与虚构订阅：`http://127.0.0.1:18881`

打开应用，选择统一身份登录，再点击测试身份页的继续按钮。添加HTTP来源时可使用 `http://127.0.0.1:18881/provider.yaml`。这个测试源只包含文档IP和虚构SS密码，不提供实际代理连接。

来源更新 → 创建方案 → 表单调整策略组及规则 → 保存草稿 → 预览 → 校验并发布 → 新建设备 → 获取链接。来源刷新不自动发布草稿；客户端刷新主订阅后获得新发布。Clash导入按钮存在，但不能据此宣称已完成真实客户端导入和替换验收。

本地脚本固定仅允许环回HTTP，同时仍完整执行OIDC。测试发行者不进入产品可执行程序；不可作为真实身份提供方。Ctrl+C停止两个子进程。状态位于 `output/dev/state`，独立密钥位于 `output/dev/master.key`；再次运行保留它们，缺失其中任意一个时拒绝自动重建。

已构建后可用 `python tools/dev.py --skip-build`。下载需要代理时可显式添加 `--proxy`，只影响本次校验器下载，不修改系统代理；Go/npm依赖下载沿用用户自己的环境。

## 手工构建与检查

```sh
npm ci --prefix web --ignore-scripts
npm run build --prefix web
python tools/fetch_validator.py
go test ./...
go vet ./...
go build -tags webui -o output/subweft ./cmd/subweft
```

Windows输出程序建议使用 `.exe` 后缀。未使用webui标签构建的服务拒绝启动，避免把缺少前端的构建误当成完整实例。前端生成文件不进入Git，必须先构建前端再构建带webui标签的程序。

## 接入真实OIDC的配置边界

初始化命令显式要求数据目录和密钥文件；serve不会重建缺失数据库或密钥。

```sh
output/subweft init --data-dir output/local-state --key-file output/local-master.key
output/subweft serve --data-dir output/local-state --key-file output/local-master.key
```

serve需要从受限运行环境提供：

| 环境键 | 含义 |
|---|---|
| SUBWEFT_BASE_URL | 明确的应用HTTPS基址，无路径 |
| SUBWEFT_LISTEN | 监听地址，默认127.0.0.1:8080 |
| SUBWEFT_OIDC_ISSUER | 精确issuer，默认要求HTTPS |
| SUBWEFT_OIDC_CLIENT_ID | OIDC客户端ID |
| SUBWEFT_OIDC_CLIENT_SECRET | 受限客户端密钥 |
| SUBWEFT_ADMIN_SUBJECTS | 明确准入的管理员sub，逗号分隔 |
| SUBWEFT_MIHOMO_PATH | 与锁定摘要相符的程序绝对路径 |
| SUBWEFT_FETCH_ALLOW_CIDRS | 可选的明确来源目标允许列表，默认不开放私网 |

回调是基址加 `/auth/callback`。非本地模式只接受HTTPS基址和issuer；`SUBWEFT_LOCAL_HTTP=true`要求应用、issuer及监听均为明确环回IP。不要把机密放在命令参数、聊天、Git或截图中。

本轮不提供生产Compose、Nginx入口、证书或Pocket ID客户端注册。现有基础设施保持原状；正式接入与切换需要完成对应验收。

## 性能基准

```sh
go run ./tools/bench -validator /absolute/path/to/mihomo -requests 200
```

工具使用虚构的10个来源、2000个SS节点、5份方案、20台设备和10并发，在原生校验后测量真实授权下载接口。只输出汇总，不输出Token或配置正文。它使用临时加密数据库，结束后清理自己的临时数据。

该基准未包含大型公共规则集、外网传输或实际代理连接，Go heap不等于RSS；不能把其结果当作完整首版性能验收。

## Linux 校验前置条件

Linux 需安装发行版 bubblewrap、util-linux（prlimit），并允许该程序建立非特权用户/文件/网络命名空间。服务启动时验证隔离可用；不满足时拒绝校验器，不降级运行。Windows 仅用于可信合成数据的本地开发，不能据此验收 Linux 隔离。
