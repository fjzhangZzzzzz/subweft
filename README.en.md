# Subweft

**A subscription and policy configuration service for Mihomo.**

[简体中文](README.md) · [Design](docs/design.md) · [Scope](docs/support-matrix.md) · [Acceptance criteria](docs/acceptance.md)

> This repository contains an approved design baseline, not a runnable application. There are no release artifacts. The features below are planned and are not suitable for production use yet.

Subweft is intended for personal and household use. It brings subscriptions, manual nodes, policy groups, rules, configuration publishing, and device links into a form-based web interface.

## Planned behavior

- Structured forms for routine and advanced settings; YAML is reserved for import, export, and read-only previews.
- Shared sources and independent configuration profiles with their own groups, ordered rules, DNS, and client settings.
- Groups can combine multiple subscriptions, tags, manual nodes, and other valid groups.
- Validated, atomic publishing with last-known-good caches, immutable revisions, and rollback.
- Independent device credentials with profile switching, suspension, revocation, and rotation.
- Native standard OIDC, with Pocket ID as the first integration to be accepted.
- A single Go service with SQLite and an embedded React UI; Nginx provides HTTPS reverse proxying.

The [support matrix](docs/support-matrix.md) defines the initial scope. Form coverage, parser acceptance, native Mihomo validation, and actual connectivity are separate claims.

The service keeps upstream credentials server-side. Revoking a device prevents future downloads; it cannot revoke proxy credentials already downloaded. Configuration changes are applied when the client refreshes its subscription.

## Deployment and status

The initial production targets are Linux amd64 and arm64. Windows is intended for development and local evaluation. Initial client acceptance is limited to a local Clash Verge installation. No installation command, image, or client workflow has been verified yet.

Subweft does not run a traffic-serving proxy core. It invokes a pinned Mihomo executable for configuration validation. The deployer owns installation, TLS, host operations, and system recovery. The application database owns business state.

This is a new public project with no imported legacy Git history, URL compatibility, or legacy authentication support. Existing deployments must import their data and enroll new device links before switching over.

## Development

See [development](docs/development.md). The checks available at this stage are:

```sh
python tools/check_docs.py
git diff --check
```

## License

Original repository content is licensed under [Apache-2.0](LICENSE). Mihomo and other dependencies retain their own licenses; distributing them requires a separate license review.
