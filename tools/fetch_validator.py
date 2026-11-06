#!/usr/bin/env python3
"""Fetch only the reviewed validator artifact for an explicit supported platform."""
from __future__ import annotations

import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import platform
import tempfile
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[1]
MAX_BYTES = 100 * 1024 * 1024


def main() -> None:
    default_os = {"Windows": "windows", "Linux": "linux"}.get(platform.system(), "unsupported")
    default_arch = {"AMD64": "amd64", "x86_64": "amd64", "aarch64": "arm64", "ARM64": "arm64"}.get(platform.machine(), "unsupported")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--platform", default=f"{default_os}-{default_arch}")
    parser.add_argument("--proxy", help="Optional HTTP(S) proxy for this download only")
    parser.add_argument("--output", type=Path, help="Default: output/tools/mihomo[.exe]")
    args = parser.parse_args()
    lock = json.loads((ROOT / "internal/validator/lock.json").read_text(encoding="utf-8"))
    if args.platform not in lock["artifacts"]:
        parser.error("platform is not in the reviewed artifact lock")
    artifact = lock["artifacts"][args.platform]
    destination = args.output or ROOT / "output/tools" / ("mihomo.exe" if args.platform.startswith("windows-") else "mihomo")
    if destination.exists():
        if destination.is_symlink() or hashlib.sha256(destination.read_bytes()).hexdigest() != artifact["binary_sha256"]:
            raise SystemExit("existing output differs; refusing to overwrite")
        print(f"Verified existing {args.platform} validator: {destination}")
        return
    handlers = [urllib.request.ProxyHandler({"http": args.proxy, "https": args.proxy})] if args.proxy else []
    opener = urllib.request.build_opener(*handlers)
    request = urllib.request.Request(artifact["url"], headers={"User-Agent": "Subweft-validator-fetch"})
    with opener.open(request, timeout=60) as response:
        archive = response.read(MAX_BYTES + 1)
    if len(archive) > MAX_BYTES or hashlib.sha256(archive).hexdigest() != artifact["archive_sha256"]:
        raise SystemExit("archive size or digest mismatch")
    if artifact["archive"] == "zip":
        with zipfile.ZipFile(io.BytesIO(archive)) as package:
            if package.namelist() != [artifact["entry"]] or package.getinfo(artifact["entry"]).file_size > MAX_BYTES:
                raise SystemExit("unexpected ZIP contents")
            binary = package.read(artifact["entry"])
    else:
        with gzip.GzipFile(fileobj=io.BytesIO(archive)) as package:
            binary = package.read(MAX_BYTES + 1)
    if len(binary) > MAX_BYTES or hashlib.sha256(binary).hexdigest() != artifact["binary_sha256"]:
        raise SystemExit("binary size or digest mismatch")
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(dir=destination.parent, prefix=".validator-", delete=False) as temporary:
        temporary.write(binary)
        temporary.flush()
        os.fsync(temporary.fileno())
        temporary_path = Path(temporary.name)
    try:
        temporary_path.chmod(0o755)
        if destination.exists():
            raise SystemExit("output appeared during download; refusing to overwrite")
        os.link(temporary_path, destination)
    finally:
        temporary_path.unlink(missing_ok=True)
    print(f"Verified {args.platform} validator: {destination}")


if __name__ == "__main__":
    main()
