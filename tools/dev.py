#!/usr/bin/env python3
"""Build and run the loopback-only development application and OIDC fixture."""
from __future__ import annotations

import argparse
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[1]


def executable(name: str) -> str:
    found = shutil.which(name)
    if not found:
        raise SystemExit(f"Required tool is not installed: {name}")
    return found


def run(args: list[str], cwd: Path = ROOT) -> None:
    subprocess.run(args, cwd=cwd, check=True)


def check_port(port: int) -> None:
    with socket.socket() as listener:
        try:
            listener.bind(("127.0.0.1", port))
        except OSError:
            raise SystemExit(f"Port {port} is in use; no service was started")


def wait_ready(url: str, child: subprocess.Popen[bytes]) -> None:
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    for _ in range(100):
        if child.poll() is not None:
            raise SystemExit("Development process exited before readiness")
        try:
            with opener.open(url, timeout=0.5) as response:
                if response.status == 200:
                    return
        except OSError:
            pass
        time.sleep(0.1)
    raise SystemExit("Development readiness timed out")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--skip-build", action="store_true", help="Use already built executables")
    parser.add_argument("--proxy", help="Optional proxy used only to fetch the pinned validator")
    args = parser.parse_args()
    for port in (18880, 18881):
        check_port(port)
    suffix = ".exe" if os.name == "nt" else ""
    output = ROOT / "output/bin"
    output.mkdir(parents=True, exist_ok=True)
    app = output / f"subweft{suffix}"
    fixture = output / f"devfixture{suffix}"
    validator = ROOT / "output/tools" / f"mihomo{suffix}"
    fetch = [sys.executable, "tools/fetch_validator.py"]
    if args.proxy:
        fetch += ["--proxy", args.proxy]
    run(fetch)
    run([sys.executable, "tools/fetch_testdata.py"])
    os.environ["CGO_ENABLED"] = "0"
    if not args.skip_build:
        go = executable("go")
        node = Path(subprocess.check_output([executable("node"), "-p", "process.execPath"], text=True).strip())
        npm_command = shutil.which("npm")
        candidates = [node.parent / "node_modules/npm/bin/npm-cli.js", node.parent.parent / "lib/node_modules/npm/bin/npm-cli.js"]
        if npm_command and Path(npm_command).resolve().suffix not in {".cmd", ".exe", ".bat"}:
            candidates.append(Path(npm_command).resolve())
        npm = next((candidate for candidate in candidates if candidate.is_file()), candidates[0])
        if not npm.is_file():
            raise SystemExit("Cannot locate bundled npm CLI; use the manual build commands in docs/running.md")
        run([str(node), str(npm), "ci", "--ignore-scripts", "--no-audit", "--no-fund"], ROOT / "web")
        run([str(node), str(npm), "run", "build"], ROOT / "web")
        run([go, "build", "-tags", "webui", "-o", str(app), "./cmd/subweft"])
        run([go, "build", "-o", str(fixture), "./tools/devfixture"])
    if not app.is_file() or not fixture.is_file():
        raise SystemExit("Build outputs are missing")
    state = ROOT / "output/dev/state"
    key = ROOT / "output/dev/master.key"
    database = state / "subweft.sqlite"
    if not database.exists() and not key.exists():
        run([str(app), "init", "--data-dir", str(state), "--key-file", str(key)])
    elif not database.exists() or not key.exists():
        raise SystemExit("State or key is missing; restore the matching pair instead of reinitializing")
    env = {**os.environ, "SUBWEFT_LISTEN": "127.0.0.1:18880", "SUBWEFT_BASE_URL": "http://127.0.0.1:18880", "SUBWEFT_LOCAL_HTTP": "true", "SUBWEFT_OIDC_ISSUER": "http://127.0.0.1:18881", "SUBWEFT_OIDC_CLIENT_ID": "subweft-local", "SUBWEFT_OIDC_CLIENT_SECRET": "fixture-client-secret-not-for-production", "SUBWEFT_ADMIN_SUBJECTS": "fixture-admin", "SUBWEFT_MIHOMO_PATH": str(validator.resolve()), "SUBWEFT_FETCH_ALLOW_CIDRS": "127.0.0.1/32"}
    children: list[subprocess.Popen[bytes]] = []
    creationflags = subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0
    try:
        idp = subprocess.Popen([str(fixture)], cwd=ROOT, env=env, creationflags=creationflags)
        children.append(idp)
        wait_ready("http://127.0.0.1:18881/.well-known/openid-configuration", idp)
        service = subprocess.Popen([str(app), "serve", "--data-dir", str(state), "--key-file", str(key)], cwd=ROOT, env=env, creationflags=creationflags)
        children.append(service)
        wait_ready("http://127.0.0.1:18880/healthz", service)
        print("Development preview: http://127.0.0.1:18880", flush=True)
        print("Use the test identity page. Only synthetic subscriptions should be used. Ctrl+C stops both processes.", flush=True)
        while all(child.poll() is None for child in children):
            time.sleep(0.5)
    except KeyboardInterrupt:
        pass
    finally:
        for child in reversed(children):
            if child.poll() is None:
                child.terminate()
                try:
                    child.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    child.kill()
                    child.wait()


if __name__ == "__main__":
    main()
