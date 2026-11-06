"""Benchmark ADR-0008 APIs using isolated, synthetic state and real native checks."""
from __future__ import annotations

import concurrent.futures
import hashlib
import http.cookiejar
import json
import math
import os
from pathlib import Path
import platform
import statistics
import subprocess
import tempfile
import time
import urllib.parse
import urllib.request

from benchmark import memory, port

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "output/editor-performance"


def summary(values):
    v = sorted(values)
    return {"n": len(v), "p50_ms": round(statistics.median(v), 2),
            "p95_ms": round(v[math.ceil(len(v) * .95) - 1], 2),
            "max_ms": round(v[-1], 2)}


def config(base, sources, groups, sets, entries):
    return {"base": base, "fallback": "g0",
            "subscriptions": [{"id": f"s{i}", "name": f"Subscription-{i}",
                               "url": f"https://source-{i}.example.com/subscription", "enabled": True,
                               "pool": "quality" if i % 2 else "quota", "interval": 3600,
                               "prefix": "", "healthCheck": False, "proxy": "DIRECT"} for i in range(sources)],
            "groups": [{"id": f"g{i}", "name": f"Group-{i}", "type": "select",
                        "pools": ["quality", "quota"], "builtins": ["DIRECT"]} for i in range(groups)],
            "ruleSets": [{"id": f"set{i}", "name": f"Purpose-{i}", "kind": "local",
                          "entries": [{"id": f"e{i}_{j}", "type": "suffix",
                                       "value": f"entry-{i}-{j}.example.com"}
                                      for j in range(entries // sets)]} for i in range(sets)],
            "rules": [{"id": f"r{i}", "type": "RULE-SET", "value": f"set{i}",
                       "target": f"g{i % groups}", "noResolve": False} for i in range(sets)]}


def run_case(name, sources, groups, sets, entries, device_count):
    root = Path(tempfile.mkdtemp(prefix=name + "-", dir=OUT))
    application = ROOT / "output/bin/subweft-editor.exe"
    fixture = ROOT / "output/bin/editor-idp.exe"
    base = "http://127.0.0.1:" + str(port())
    issuer = "http://127.0.0.1:" + str(port())
    flags = subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0
    children = []
    direct = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    client = urllib.request.build_opener(urllib.request.ProxyHandler({}),
                                       urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    csrf = ""

    def request(path, body=None, method=None):
        req = urllib.request.Request(base + "/api/v1/" + path,
                                     data=None if body is None else json.dumps(body).encode(), method=method,
                                     headers={"Content-Type": "application/json", "Origin": base, "X-CSRF-Token": csrf})
        start = time.perf_counter()
        with client.open(req, timeout=120) as response:
            raw = response.read()
        elapsed = (time.perf_counter() - start) * 1000
        return json.loads(raw), elapsed, len(raw)

    def ready(url, process):
        for _ in range(200):
            if process.poll() is not None:
                raise RuntimeError("owned process exited")
            try:
                with direct.open(url, timeout=.5) as r:
                    if r.status == 200:
                        return
            except OSError:
                pass
            time.sleep(.05)
        raise RuntimeError("readiness timeout")

    with (root / "process.log").open("wb") as log:
        env = {**os.environ, "SUBWEFT_LISTEN": urllib.parse.urlsplit(base).netloc,
               "SUBWEFT_BASE_URL": base, "SUBWEFT_LOCAL_HTTP": "true", "SUBWEFT_OIDC_ISSUER": issuer,
               "SUBWEFT_OIDC_CLIENT_ID": "subweft-local", "SUBWEFT_OIDC_CLIENT_SECRET": "fixture-client-secret-not-for-production",
               "SUBWEFT_ADMIN_SUBJECTS": "fixture-admin", "SUBWEFT_MIHOMO_PATH": str(ROOT / "output/tools/mihomo.exe")}
        try:
            subprocess.run([str(application), "init", "--data-dir", str(root / "state"), "--key-file", str(root / "key")],
                           cwd=ROOT, env=env, stdout=log, stderr=log, check=True, creationflags=flags)
            idp = subprocess.Popen([str(fixture), "--listen", urllib.parse.urlsplit(issuer).netloc, "--base-url", base],
                                   cwd=ROOT, env=env, stdout=log, stderr=log, creationflags=flags)
            children.append(idp)
            ready(issuer + "/.well-known/openid-configuration", idp)
            started = time.perf_counter()
            app = subprocess.Popen([str(application), "serve", "--data-dir", str(root / "state"), "--key-file", str(root / "key")],
                                   cwd=ROOT, env=env, stdout=log, stderr=log, creationflags=flags)
            children.append(app)
            ready(base + "/healthz", app)
            startup = (time.perf_counter() - started) * 1000
            with client.open(base + "/auth/login", timeout=30) as r:
                query = urllib.parse.urlsplit(r.url).query
                r.read()
            with client.open(urllib.request.Request(issuer + "/approve", data=query.encode()), timeout=30) as r:
                r.read()
            initial = request("state")[0]
            csrf = initial["csrf"]
            c = config(initial["draft"]["config"]["base"], sources, groups, sets, entries)
            revision = initial["draft"]["revision"]
            samples = {"save": [], "check": [], "publish": []}
            for i in range(20):
                c["subscriptions"][0]["prefix"] = f"revision-{i}-"
                draft, ms, _ = request("draft", {"revision": revision, "config": c}, "PUT")
                revision = draft["revision"]
                samples["save"].append(ms)
                checked, ms, _ = request("check", {"revision": revision}, "POST")
                if checked["problems"]:
                    raise RuntimeError("synthetic configuration failed native check")
                samples["check"].append(ms)
                _, ms, _ = request("publish", {"revision": revision, "note": "synthetic performance test"}, "POST")
                samples["publish"].append(ms)
                if i in (0, 9, 19):
                    print(f"{name}: {i + 1}/20 native-validated releases", flush=True)
            links = []
            for i in range(device_count):
                device = request("devices", {"action": "save", "device": {"name": f"Device-{i}"}}, "POST")[0]
                links.append(request("devices/" + device["id"] + "/link")[0]["url"])
            for link in links:
                with direct.open(link, timeout=60) as r:
                    expected = hashlib.sha256(r.read()).digest()
            time.sleep(5)
            idle = memory(app.pid)[0]
            states = [request("state") for _ in range(20)]
            results = {"name": name, "sources": sources, "groups": groups, "sets": sets, "entries": entries,
                       "devices": device_count, "releases": 20, "startup_ms": round(startup, 2),
                       "idle_rss_mib": round(idle / 2**20, 2), "state": summary([x[1] for x in states]),
                       "state_bytes": states[-1][2], "transactions": {k: summary(v) for k, v in samples.items()},
                       "first_transaction_ms": {k: round(v[0], 2) for k, v in samples.items()}, "downloads": []}

            def download(i):
                # Each worker owns its opener. Timing includes connection and body read.
                opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
                start = time.perf_counter()
                try:
                    with opener.open(links[i % len(links)], timeout=60) as response:
                        body = response.read()
                    return (time.perf_counter() - start) * 1000, len(body), hashlib.sha256(body).digest() == expected
                except Exception:
                    return (time.perf_counter() - start) * 1000, 0, False

            for concurrency in (1, 10, 30):
                start = time.perf_counter()
                with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as pool:
                    rows = list(pool.map(download, range(200)))
                seconds = time.perf_counter() - start
                results["downloads"].append({"concurrency": concurrency, **summary([r[0] for r in rows]),
                                             "requests_per_second": round(len(rows) / seconds, 2),
                                             "errors": sum(not r[2] for r in rows), "config_bytes": rows[0][1]})
                print(f"{name}: download concurrency {concurrency} finished", flush=True)
            results["final_rss_mib"], results["process_peak_rss_mib"] = [round(x / 2**20, 2) for x in memory(app.pid)]
            results["database_bytes"] = sum(p.stat().st_size for p in (root / "state").glob("subweft.sqlite*"))
            results["validation_child_memory"] = "not measured"
            return results
        finally:
            for child in reversed(children):
                if child.poll() is None:
                    child.terminate()
                    try:
                        child.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        child.kill()
                        child.wait()


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    app = ROOT / "output/bin/subweft-editor.exe"
    report = {"date": time.strftime("%Y-%m-%d %H:%M:%S %z"), "os": platform.platform(),
              "logical_processors": os.cpu_count(), "python": platform.python_version(),
              "build": json.loads(subprocess.check_output([str(app), "version"], text=True)),
              "binary_sha256": hashlib.sha256(app.read_bytes()).hexdigest(), "cases": []}
    for case in [("household", 5, 6, 10, 500, 10), ("expanded", 20, 20, 50, 5000, 20), ("stress", 50, 30, 100, 10000, 50)]:
        report["cases"].append(run_case(*case))
        (OUT / "results.json").write_text(json.dumps(report, indent=2, ensure_ascii=False), encoding="utf-8")
    print("Report: output/editor-performance/results.json", flush=True)


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        # Request exceptions may contain device tokens; never print them verbatim.
        raise SystemExit("Benchmark failed: " + type(error).__name__) from None
