"""Verify the packaged single-configuration editor with synthetic OIDC and recovery."""
import argparse
import concurrent.futures
import http.cookiejar
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

from benchmark import port
from editor_benchmark import config

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--app', type=Path, required=True)
    parser.add_argument('--fixture', type=Path, required=True)
    parser.add_argument('--work-dir', type=Path, default=ROOT / 'output')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    app, fixture = args.app.resolve(), args.fixture.resolve()
    root = Path(tempfile.mkdtemp(prefix='editor-smoke-', dir=args.work_dir))
    state, key = root / 'state', root / 'master.key'
    base, issuer = ['http://127.0.0.1:' + str(port()) for _ in range(2)]
    suffix = '.exe' if os.name == 'nt' else ''
    env = {**os.environ, 'SUBWEFT_LISTEN': urllib.parse.urlsplit(base).netloc,
           'SUBWEFT_BASE_URL': base, 'SUBWEFT_LOCAL_HTTP': 'true', 'SUBWEFT_OIDC_ISSUER': issuer,
           'SUBWEFT_OIDC_CLIENT_ID': 'subweft-local', 'SUBWEFT_OIDC_CLIENT_SECRET': 'fixture-client-secret-not-for-production',
           'SUBWEFT_ADMIN_SUBJECTS': 'fixture-admin', 'SUBWEFT_MIHOMO_PATH': str(ROOT / 'output/tools' / ('mihomo' + suffix))}
    flags = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0
    direct = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    client = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    csrf, children, stage = '', [], 'initialize'

    def api(path, body=None, method=None, expected=200):
        req = urllib.request.Request(base + '/api/v1/' + path, method=method,
                                     data=None if body is None else json.dumps(body).encode(),
                                     headers={'Content-Type': 'application/json', 'Origin': base, 'X-CSRF-Token': csrf})
        try:
            with client.open(req, timeout=60) as r:
                code, data = r.status, r.read()
        except urllib.error.HTTPError as e:
            code, data = e.code, e.read()
        if code != expected:
            raise RuntimeError('unexpected API status')
        return json.loads(data) if data else None

    def download(link, expected=200):
        try:
            with direct.open(link, timeout=30) as r:
                code, data = r.status, r.read()
        except urllib.error.HTTPError as e:
            code, data = e.code, e.read()
        if code != expected:
            raise RuntimeError('unexpected device status')
        return data

    def ready(url, child):
        for _ in range(200):
            if child.poll() is not None:
                raise RuntimeError('owned process exited')
            try:
                with direct.open(url, timeout=.5) as r:
                    if r.status == 200:
                        return
            except OSError:
                pass
            time.sleep(.05)
        raise RuntimeError('readiness timeout')

    with (root / 'process.log').open('wb') as log:
        def command(*argv):
            subprocess.run([str(app), *map(str, argv)], cwd=ROOT, env=env, stdout=log, stderr=log, check=True, creationflags=flags)

        def start(directory):
            child = subprocess.Popen([str(app), 'serve', '--data-dir', str(directory), '--key-file', str(key)],
                                     cwd=ROOT, env=env, stdout=log, stderr=log, creationflags=flags)
            children.append(child)
            ready(base + '/healthz', child)
            return child

        try:
            command('init', '--data-dir', state, '--key-file', key)
            idp = subprocess.Popen([str(fixture), '--listen', urllib.parse.urlsplit(issuer).netloc, '--base-url', base],
                                   cwd=ROOT, env=env, stdout=log, stderr=log, creationflags=flags)
            children.append(idp)
            ready(issuer + '/.well-known/openid-configuration', idp)
            service = start(state)
            stage = 'OIDC authorization redirect'
            with client.open(base + '/auth/login') as r:
                query = urllib.parse.urlsplit(r.url).query
                r.read()
            stage = 'OIDC approval and callback'
            with client.open(urllib.request.Request(issuer + '/approve', data=query.encode())) as r:
                r.read()
            stage = 'authenticated state'
            initial = api('state')
            stage = 'CSRF rejection response'
            api('draft', initial['draft'], 'PUT', expected=403)
            csrf = initial['csrf']
            stage = 'native check and publish'
            c = config(initial['draft']['config']['base'], 2, 2, 2, 20)
            c['ruleSets'][0]['entries'].append({'id': 'ip-entry', 'type': 'ip', 'value': '192.0.2.0/24'})
            draft = api('draft', {'revision': initial['draft']['revision'], 'config': c}, 'PUT')
            checked = api('check', {'revision': draft['revision']}, 'POST')
            if checked['problems']:
                raise RuntimeError('native check failed')
            api('publish', {'revision': draft['revision']}, 'POST')
            device = api('devices', {'action': 'save', 'device': {'name': 'Synthetic device'}}, 'POST')
            link = api('devices/' + device['id'] + '/link')['url']
            original = download(link)
            if b'Purpose-0-ipcidr' not in original or b'Purpose-0-domain' not in original:
                raise RuntimeError('mixed provider rendering missing')
            stage = 'failure isolation and concurrency'
            c['groups'][0]['pools'], c['groups'][0]['builtins'] = [], []
            draft = api('draft', {'revision': draft['revision'], 'config': c}, 'PUT')
            if not api('check', {'revision': draft['revision']}, 'POST')['problems']:
                raise RuntimeError('invalid group accepted')
            api('publish', {'revision': draft['revision']}, 'POST', expected=422)
            with concurrent.futures.ThreadPoolExecutor(max_workers=10) as pool:
                if any(data != original for data in pool.map(lambda _: download(link), range(50))):
                    raise RuntimeError('active release changed on failure')
            stage = 'device authorization'
            device = api('devices', {'action': 'pause', 'device': device}, 'POST')
            download(link, expected=404)
            device = api('devices', {'action': 'resume', 'device': device}, 'POST')
            if download(link) != original:
                raise RuntimeError('resume changed content')
            device = api('devices', {'action': 'rotate', 'device': device}, 'POST')
            download(link, expected=404)
            link = api('devices/' + device['id'] + '/link')['url']
            stage = 'restore to draft'
            draft = api('restore', {'revision': draft['revision'], 'number': 1}, 'POST')
            if download(link) != original or api('state')['check'] is not None:
                raise RuntimeError('restore changed active release or kept check')
            stage = 'CLI backup and recovery'
            service.terminate()
            service.wait(timeout=10)
            backup, restored = root / 'backup.sqlite', root / 'restored'
            command('backup', '--data-dir', state, '--key-file', key, '--output', backup)
            command('restore', '--backup-file', backup, '--data-dir', restored, '--key-file', key)
            service = start(restored)
            if download(link) != original:
                raise RuntimeError('recovered content differs')
            report = {'editor_smoke': 'passed', 'packaged_build': json.loads(subprocess.check_output([str(app), 'version'], text=True)),
                      'checks': ['OIDC', 'CSRF', 'native-check', 'mixed-providers', 'publish', 'failure-preserves-active',
                                 '50-downloads-10-concurrent', 'pause-resume', 'token-rotation', 'restore-to-draft', 'CLI-backup-restore']}
            args.output.parent.mkdir(parents=True, exist_ok=True)
            args.output.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
            print(json.dumps(report), flush=True)
        except Exception as e:
            raise SystemExit('Editor smoke failed at ' + stage + ': ' + type(e).__name__) from None
        finally:
            for child in reversed(children):
                if child.poll() is None:
                    child.terminate()
                    try:
                        child.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        child.kill()
                        child.wait()


if __name__ == '__main__':
    main()
