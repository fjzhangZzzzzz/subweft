# ADR-0012：正式版容器镜像发布到公开 GHCR

状态：已接受。

## 背景

Subweft 源码仓库公开，control-01 以容器运行。只保留源代码而将正式镜像留在本机，会使重建依赖现场镜像；将镜像保存在私有 GHCR 又要求 Dagu 持续维护包读取凭据。

## 决策

- `Source checks` 成功后，只对已合入 `main`、且版本与 `internal/buildinfo/VERSION` 一致的稳定 SemVer tag 构建 Linux amd64 运行镜像。
- 使用 `deploy/Dockerfile` 构建 application-only 镜像；不打包 Mihomo、SQLite、订阅、凭据或运行配置。Mihomo 仍按 `internal/validator/lock.json` 独立锁定并以只读挂载。
- 发布到 `ghcr.io/fjzhangzzzzzz/subweft:vX.Y.Z`，添加 `org.opencontainers.image.source` 关联公开源码仓库；不发布 `latest` 或其他浮动标签。构建 workflow 仅有局部 `packages: write` 权限，使用 `GITHUB_TOKEN`。
- 首次发布的 GHCR 包默认是私有的；维护者须在 GitHub 包设置中将包明确改为 Public，并验证未登录时可以拉取后，homelab-infra 才登记 registry digest、image ID 和源 tag，并以 digest 部署；不纳入备份。
- workflow 自动跟随成功的版本 tag 检查发布。首次为已有 tag 补发时，维护者手动运行发布 workflow；它须验证该 tag 的 Source checks 已成功。

## 后果

Dagu 可以匿名拉取已锁定的公开镜像，无需 Global Secret Registry PAT；灾难恢复不依赖 control-01 的 Docker 缓存。公开 GHCR 包无法改回私有，首次改为 Public 前确认镜像中没有订阅、密钥或其他运行数据。镜像发布不等于生产部署或验收；版本切换仍按 homelab-infra 的部署与恢复流程执行。
