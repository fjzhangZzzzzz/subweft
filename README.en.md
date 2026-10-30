# Subweft

A personal and household **Mihomo configuration editor**. [中文](README.md)

Candidate: **v0.3.0-rc.1**, implementing the reviewed single-configuration model.

Maintain subscriptions, policy groups, purpose-based rule sets and ordered routing rules through structured forms. Review a redacted Git-style diff, run the pinned native validator, and explicitly publish. Devices download the active immutable release through independent authorizations.

Subscriptions belong to one of two pools. Groups support select / url-test and DIRECT / REJECT. Local purpose-based rule sets may mix domains and CIDRs; the compiler emits readable providers by behavior. The server does not fetch upstream subscriptions. Manual nodes, nested groups and multiple profiles are not supported. Base DNS and ports are imported once and remain read-only.

Run `python tools/dev.py` and open <http://127.0.0.1:18884/> for a loopback-only synthetic OIDC environment. See [local setup](docs/running.md), [supported scope](docs/support-matrix.md) and [human acceptance](docs/human-acceptance.md).

Linux amd64/arm64 and Windows amd64 application archives exclude Mihomo. Supply the separately pinned validator and deployment-owned GeoDB. SQLite owns business state; keep its encryption key separate. Use a fresh data directory for this rewrite; see [cutover and rollback](docs/editor-cutover.md).

The [performance baseline](docs/performance-report.md) identifies large-history response sizes and serialized downloads as known bottlenecks. Real Pocket ID, upstream connectivity, client acceptance and production cutover remain separate acceptance steps.

[Releases](https://github.com/fjzhangZzzzzz/subweft/releases) · [Apache-2.0](LICENSE) · [Third-party notices](THIRD_PARTY.md)
