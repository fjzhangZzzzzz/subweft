#!/usr/bin/env python3
"""Check tracked documentation links and repository admission without networking."""

from __future__ import annotations

import re
import subprocess
import sys
from pathlib import Path
from urllib.parse import unquote, urlsplit


ROOT = Path(__file__).resolve().parents[1]
LINK = re.compile(r"!?\[[^\]\n]*\]\(([^\s)]+)(?:\s+[^)]*)?\)")
FORBIDDEN_PARTS = {"node_modules", "__pycache__", ".playwright-cli"}
RUNTIME_ROOTS = {"data", "state", "cache", "snapshots", "output"}
FORBIDDEN_NAMES = {"runtime.env", "compose.env", "oidc-client-secret"}
FORBIDDEN_SUFFIXES = {".db", ".sqlite", ".sqlite3", ".key", ".pem", ".exe", ".pyc"}


def main() -> int:
    result = subprocess.run(
        ["git", "ls-files", "--stage", "-z"], cwd=ROOT, check=True, capture_output=True
    )
    errors: list[str] = []
    files: list[Path] = []
    for entry in result.stdout.decode("utf-8").split("\0"):
        if not entry:
            continue
        metadata, name = entry.split("\t", 1)
        mode = metadata.split()[0]
        rel = Path(name)
        file = ROOT / rel
        files.append(file)
        if mode not in {"100644", "100755"}:
            errors.append(f"{name}: links and submodules are not admitted")
            continue
        if not file.is_file() or file.is_symlink():
            errors.append(f"{name}: tracked file is missing or is a link")
            continue
        if rel.parts[0] in RUNTIME_ROOTS or any(part in FORBIDDEN_PARTS for part in rel.parts):
            errors.append(f"{name}: runtime or local-tooling directory is tracked")
        if file.suffix.lower() in FORBIDDEN_SUFFIXES or re.search(r"\.(?:db|sqlite3?)-(?:wal|shm|journal)$", name):
            errors.append(f"{name}: runtime or generated file is tracked")
        if file.name in FORBIDDEN_NAMES:
            errors.append(f"{name}: deployment secrets or environment are tracked")
        if file.name.startswith(".env") and file.name != ".env.example":
            errors.append(f"{name}: runtime environment file is tracked")
        if file.suffix.lower() not in {".md", ".py", ".yml", ".yaml", ".json", ".toml"} and file.name not in {"LICENSE", ".gitignore", ".gitattributes"}:
            continue
        try:
            text = file.read_text(encoding="utf-8")
        except UnicodeError:
            errors.append(f"{name}: text is not UTF-8")
            continue
        if file.suffix.lower() != ".md":
            continue
        for match in LINK.finditer(text):
            target = match.group(1).strip("<>")
            url = urlsplit(target)
            if url.scheme in {"https", "http", "mailto"}:
                continue
            if url.scheme or url.netloc or target.startswith(("/", "\\")):
                errors.append(f"{name}: nonportable link {target}")
                continue
            if not url.path:
                continue
            destination = (file.parent / unquote(url.path)).resolve()
            if not destination.is_relative_to(ROOT.resolve()) or not destination.is_file():
                errors.append(f"{name}: broken local link {target}")
    if not files:
        errors.append("no tracked files; stage the intended source tree before checking")
    for error in errors:
        print(error, file=sys.stderr)
    if errors:
        return 1
    print(f"Documentation and admission checks passed for {len(files)} tracked files.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
