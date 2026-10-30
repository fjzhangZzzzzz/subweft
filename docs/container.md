# Linux 容器安装

> 本文包含旧候选部署记录。ADR-0008 新编辑器尚未重新完成容器验收；不要将下述旧业务 API 和性能通过结论用于本版。当前本机入口见 human-acceptance.md，生产切换见 editor-cutover.md。

本入口面向 Linux amd64/arm64。镜像只含 Subweft 和系统运行依赖，Mihomo 按[锁定文件](../internal/validator/lock.json)单独取得并以只读文件挂载。当前容器验证由原生 CI 执行，未向生产部署。

## 隔离边界

[Dockerfile](../deploy/Dockerfile)固定 Debian 基镜像摘要及软件源快照；构建使用宿主系统 CA 证书包引导 HTTPS。运行以 UID/GID 10001、只读根、全部 capability 丢弃、no-new-privileges、PID/内存限制及临时内存卷进行。主密钥和校验核心分别只读挂载，SQLite 单独持久化。

Docker 默认策略禁止嵌套命名空间，因此提供[专用 seccomp](../deploy/seccomp.json)和[专用 AppArmor](../deploy/subweft.apparmor)，只应用于本容器。seccomp 保留固定 Moby 默认拒绝策略，额外允许 bubblewrap 所需的 user namespace、namespace clone/setns、mount/umount/pivot_root 和 hostname 系统调用；初始容器没有宿主 capability，挂载能力仅在嵌套 user namespace 内有效。AppArmor 允许该容器创建 user namespace 和私有挂载，仍拒绝写入宿主内核控制路径。不得使用 privileged、SYS_ADMIN、unconfined、共享宿主网络或全局关闭 AppArmor/userns 限制作为替代。

Mihomo 每次校验单独创建无网络命名空间，只能看到当次配置、依赖资源、固定核心及静态启动器；/proc 为空，不暴露宿主进程。CPU/地址空间/文件大小/文件描述符和 96 进程硬限制仍生效；失败不发布。校验目录必须在 tmpfs，崩溃不在持久卷留下明文配置。

[容器检查工具](../tools/container_check.py)使用独立合成数据，原生运行文件/网络/进程限制探针、发布、一致性恢复及完整性能场景。amd64/arm64 分别在对应原生免费 runner 上执行，不使用模拟结果替代。

## 构建和准备

需要 Linux Docker Engine、Compose 插件、AppArmor（支持 userns 规则）和可建立非特权 user namespace 的内核。当前原生验证系统为 Ubuntu 24.04；其他发行版需运行同一隔离探针，不推导通过。本步骤是安装说明，不会自动修改任何现有生产主机。

从经过校验的对应架构候选包解压，在包根目录执行：

```sh
mkdir -p output/container
cp subweft output/container/subweft
cp /etc/ssl/certs/ca-certificates.crt output/container/ca-certificates.crt
docker build --target runtime -f deploy/Dockerfile -t subweft-candidate:local .
docker image inspect --format '{{.Id}}' subweft-candidate:local
python3 tools/fetch_validator.py
```

源码构建先执行 `npm ci --prefix web --ignore-scripts && npm run build --prefix web`，再以 `CGO_ENABLED=0 go build -tags webui -o output/container/subweft ./cmd/subweft` 生成同架构静态程序。记录实际镜像 ID，不在正式运行中使用可变 tag。镜像使用固定 Debian 快照，更新依赖需重新评审与验证；不能认为固定摘要意味着永久没有漏洞。

只在已核对的验收主机加载专用策略：`sudo apparmor_parser -r deploy/subweft.apparmor`。不得覆盖其他产品 profile。部署包内的 `deploy/seccomp.json` 必须随 Compose 保持一致。

下面以 `/var/lib/subweft` 和 `/etc/subweft` 为例；部署方可映射到自己的规范路径：

```sh
sudo install -d -m 0700 -o 10001 -g 10001 /var/lib/subweft /etc/subweft/secrets
```

以记录的镜像 ID 运行一次 `init`，仅此初始化步骤允许写入独立密钥目录：

```sh
docker run --rm --read-only --user 10001:10001 --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --mount type=bind,source=/var/lib/subweft,target=/var/lib/subweft \
  --mount type=bind,source=/etc/subweft/secrets,target=/init-secrets \
  IMAGE_ID init --data-dir /var/lib/subweft --key-file /init-secrets/master.key
```

`init` 遇到已有状态或密钥会拒绝覆盖。现有实例必须恢复匹配的数据和密钥，不能重新初始化。

## OIDC、运行配置与 Nginx

按[真实身份验收](human-acceptance.md#真实-pocket-id--passkey)注册 Pocket ID 客户端，回调固定为 `https://新子域名/auth/callback`。从[示例](../deploy/runtime.env.example)创建 `/etc/subweft/runtime.env`；仅包含基址、issuer、client ID、管理员 sub 和必要允许列表。把客户端 secret 单独保存为 `/etc/subweft/secrets/oidc-client-secret`，owner 10001、权限 0600；主密钥保持相同限制。不要同时设置 secret 与 secret-file 两种输入。

在受限的 Compose 参数文件中设置以下机器键，路径均为绝对路径：

| 键 | 示例含义 |
|---|---|
| SUBWEFT_IMAGE | 上述实际镜像 ID 或正式已审核镜像摘要 |
| SUBWEFT_ENV_FILE | `/etc/subweft/runtime.env` |
| SUBWEFT_DATA_DIR | `/var/lib/subweft` |
| SUBWEFT_KEY_FILE | `/etc/subweft/secrets/master.key` |
| SUBWEFT_OIDC_SECRET_FILE | `/etc/subweft/secrets/oidc-client-secret` |
| SUBWEFT_MIHOMO_FILE | 下载且校验过的核心绝对路径 |

先用 `docker compose --env-file /etc/subweft/compose.env -f deploy/compose.yml config --quiet` 校验，再使用同一参数 `up -d`。容器只把 18880 端口映射到宿主回环地址。通过 `http://127.0.0.1:18880/healthz` 核对状态、版本、源码 commit 和 dirty 标志；健康检查只证明服务可用，不代替业务验收。启动失败时核对受限日志中的固定错误码，不导出完整环境或进程参数。

[Nginx 示例](../deploy/nginx.conf.example)放入现有新子域名 HTTPS server。证书仍归部署方原有 ACME；不申请或替换证书。Nginx 仅反代，管理端 OIDC 和设备所有资源授权均归应用。示例关闭可能包含 token/code 的请求日志；部署方使用不含敏感路径的健康指标。不要在设备路径添加 IdP 登录跳转、共享缓存或静态 alias。

## 备份恢复与升级

日常使用“系统 → 下载加密备份”；也可在现行容器中运行 `backup --data-dir /var/lib/subweft --key-file /run/secrets/master.key --output /var/lib/subweft/backup.sqlite`，输出必须是尚不存在的绝对路径。该快照已包含全部版本化资源和 SQLite 一致状态，不包含独立主密钥。把数据快照、单独保护的主密钥、OIDC secret、运行参数、候选程序/镜像 ID 和锁定核心纳入部署方既有备份；密钥不能只存于其自身加密数据库中。

恢复时创建新的空数据目录，使用**备份对应版本**及原密钥运行 `restore --backup-file /restore/backup.sqlite --data-dir /var/lib/subweft --key-file /run/secrets/master.key`。备份以只读挂载提供，恢复目标独立可写；数据库存在时拒绝覆盖，密钥错误、完整性错误或解密失败必须停止。随后启动隔离实例，验证方案、历史、配置及资源下载和设备撤权。不要让两个实例同时打开同一个 SQLite 目录。

升级前备份。先验证新镜像/程序和对应隔离策略，停止当前单个应用容器后由 Compose 更新，复用原数据及主密钥，检查版本、业务和下载。当前加密 release 索引把数据库 schema 从 1 升为 2；旧 schema-1 程序不能直接降级打开 schema-2 数据库。需要回退程序时，从升级前同一备份恢复到新目录，再由部署方切换到该目录，保留原目录供诊断。方案的“历史回滚”只切换业务发布，与程序/数据库降级是不同动作。

容器卷、密钥、证书、旧服务均不会被这些示例自动清理。完成[人工验收](human-acceptance.md)后另行评审正式切换。
