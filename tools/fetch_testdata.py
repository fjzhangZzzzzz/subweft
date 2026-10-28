"""Fetch a pinned Apache-2.0 test fixture; never production geolocation data."""
import base64
import hashlib
import json
from pathlib import Path
import urllib.request

COMMIT = "d1551916cff0712d12ff602c309234471b0402a2"
SHA256 = "fc12d6470b35b70bd3e4534dd433d3e262a6c73f7682f85c344e832d4190f02b"
TARGET = Path(__file__).resolve().parents[1] / "output/tools/GeoIP2-Country-Test.mmdb"


def main():
    if TARGET.exists() and hashlib.sha256(TARGET.read_bytes()).hexdigest() == SHA256:
        print("Pinned MMDB test fixture already verified")
        return
    urls = [
        f"https://raw.githubusercontent.com/maxmind/MaxMind-DB/{COMMIT}/test-data/GeoIP2-Country-Test.mmdb",
        f"https://api.github.com/repos/maxmind/MaxMind-DB/contents/test-data/GeoIP2-Country-Test.mmdb?ref={COMMIT}",
    ]
    for index, url in enumerate(urls):
        try:
            request = urllib.request.Request(url, headers={"User-Agent": "Subweft-test-fixtures"})
            with urllib.request.urlopen(request, timeout=30) as response:
                body = response.read(100000)
            if index:
                body = base64.b64decode(json.loads(body)["content"])
            if hashlib.sha256(body).hexdigest() != SHA256:
                raise ValueError("fixture_digest_mismatch")
            TARGET.parent.mkdir(parents=True, exist_ok=True)
            TARGET.write_bytes(body)
            print("Pinned MMDB test fixture verified")
            return
        except Exception:
            continue
    raise SystemExit("MMDB test fixture unavailable or digest mismatch")


if __name__ == "__main__":
    main()
