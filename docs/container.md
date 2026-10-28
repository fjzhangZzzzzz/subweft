# Linux 容器安装

本入口面向 Linux amd64/arm64。镜像只含 Subweft 和系统运行依赖，Mihomo 按[锁定文件](../internal/validator/lock.json)单独取得并以只读文件挂载。当前容器验证由原生 CI 执行，未向生产部署。

## 隔离边界

[Dockerfile](../deploy/Dockerfile)固定 Debian 基镜像摘要及软件源快照；构建使用宿主系统 CA 证书包引导 HTTPS。运行以 UID/GID 10001、只读根、全部 capability 丢弃、no-new-privileges、PID/内存限制及临时内存卷进行。主密钥和校验核心分别只读挂载，SQLite 单独持久化。

Docker 默认策略禁止嵌套命名空间，因此提供[专用 seccomp](../deploy/seccomp.json)和[专用 AppArmor](../deploy/subweft.apparmor)，只应用于本容器。seccomp 保留固定 Moby 默认拒绝策略，额外允许 bubblewrap 所需的 user namespace、namespace clone/setns、mount/umount/pivot_root 和 hostname 系统调用；初始容器没有宿主 capability，挂载能力仅在嵌套 user namespace 内有效。AppArmor 允许该容器创建 user namespace 和私有挂载，仍拒绝写入宿主内核控制路径。不得使用 privileged、SYS_ADMIN、unconfined、共享宿主网络或全局关闭 AppArmor/userns 限制作为替代。

Mihomo 每次校验单独创建无网络命名空间，只能看到当次配置、依赖资源、固定核心及静态启动器；/proc 为空，不暴露宿主进程。CPU/地址空间/文件大小/文件描述符和 96 进程硬限制仍生效；失败不发布。校验目录必须在 tmpfs，崩溃不在持久卷留下明文配置。

[容器检查工具](../tools/container_check.py)使用独立合成数据，原生运行文件/网络/进程限制探针、发布和一致性恢复测试。正式配置、Pocket ID 注册及 Nginx 示例将在候选验收入口提供。
