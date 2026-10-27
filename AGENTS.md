# Subweft contributor instructions

Subweft is a personal/household Mihomo configuration product, not a hosting or billing platform. Prefer Chinese explanations; preserve upstream protocol and field names. See [docs/README.md](docs/README.md) before making changes.

## Authority and scope

- [docs/design.md](docs/design.md) defines product behavior, [docs/architecture.md](docs/architecture.md) defines technical boundaries, and [docs/support-matrix.md](docs/support-matrix.md) defines coverage. Record durable changes of direction in an ADR.
- The application database is the single authority for business state. Deployment owns binaries, runtime configuration, secrets and infrastructure. Do not add bidirectional Git synchronization.
- A new public repository starts with a new history. Do not import a legacy `.git`, private runtime data, old credentials or deployment inventory.
- No legacy routes, CLI compatibility, Nginx business authentication, conversion SaaS, arbitrary user scripts, or traffic-serving proxy core.

## Implementation invariants

- All routine business edits, including supported advanced fields, must work through structured forms. A YAML editor is not a substitute.
- Preserve field types and validate on the server. Reject unsupported inputs explicitly; do not silently discard fields or nodes.
- Keep acquisition, normalized sources, profile compilation, immutable releases, and device authorization separate. Prevent circular references and cross-profile leakage.
- Failures preserve the active release. Never fetch upstream synchronously while serving a device download.
- Encrypt recoverable secrets. Keep the key outside SQLite; never include real secrets in fixtures, logs, errors, URLs in documentation, Git, issues or test output.
- OIDC uses issuer plus `sub`, not mutable email/display name, as identity. First login does not grant administration. Every management mutation needs authorization and CSRF protection.
- Enforce resource limits, SSRF controls on every outbound connection and redirect, safe import sizes, and constrained validator subprocess execution.
- Do not log device URLs, tokens, provider content, sensitive parameters or validator output verbatim.
- Use established libraries, locked dependencies and pinned validator artifacts. Review license compatibility before reuse, particularly Mihomo's GPLv3 code and binaries.

## Verification and release

- Use [docs/acceptance.md](docs/acceptance.md). Tests should exercise behavior and failure boundaries, not mirror implementation details.
- Distinguish supported forms, successful render, native validation, live protocol connectivity and real client acceptance. Never infer one from another.
- Check tracked files for runtime data and private identifiers before publishing. Public examples use reserved domains and documentation IP ranges.
- No production changes during development. Prepare a concrete migration/cutover plan and preserve the old service until the agreed acceptance passes.
- Report source committed, pushed, built, released, deployed and accepted separately. Do not advertise planned capabilities as implemented.
- CI uses free infrastructure only. Do not enable paid runners or add costs.
