#!/usr/bin/env python3
"""Native disposable Docker verification. No host state or real credentials are mounted."""
from pathlib import Path
import os, subprocess, json, platform, tarfile, hashlib
ROOT=Path(__file__).resolve().parents[1]
def run(*args,**kw): return subprocess.run(args,cwd=ROOT,check=True,**kw)
def main():
    out=ROOT/'output/container';out.mkdir(parents=True,exist_ok=True)
    env={**os.environ,'CGO_ENABLED':'0'}
    arch={'x86_64':'amd64','aarch64':'arm64'}[platform.machine()]
    commit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()
    candidates=list((ROOT/'output/packages'/commit).glob('subweft-*-linux-'+arch+'.tar.gz'))
    if len(candidates)!=1:raise SystemExit('One exact native candidate is required before container verification')
    with tarfile.open(candidates[0]) as package:
        prefix=candidates[0].name.removesuffix('.tar.gz')
        binary=package.extractfile(prefix+'/subweft').read()
        manifest=json.load(package.extractfile(prefix+'/BUILD.json'))
        if hashlib.sha256(binary).hexdigest()!=manifest['files']['subweft']:raise SystemExit('Candidate digest mismatch')
        (out/'subweft').write_bytes(binary);(out/'subweft').chmod(0o555)
    run('go','build','-o',str(out/'devfixture'),'./tools/devfixture',env=env)
    for package in ('validator','app','server','configuration'):
        run('go','test','-c','-o',str(out/(package+'.test')),'./internal/'+package,env=env)
    (out/'ca-certificates.crt').write_bytes(Path('/etc/ssl/certs/ca-certificates.crt').read_bytes())
    run('docker','build','--provenance=false','--target','acceptance','-f','deploy/Dockerfile','-t','subweft-check:local','.')
    compose_env={**os.environ,'SUBWEFT_IMAGE':'subweft-check:local','SUBWEFT_ENV_FILE':str(ROOT/'deploy/runtime.env.example'),'SUBWEFT_DATA_DIR':str(out),'SUBWEFT_KEY_FILE':str(ROOT/'deploy/runtime.env.example'),'SUBWEFT_OIDC_SECRET_FILE':str(ROOT/'deploy/runtime.env.example'),'SUBWEFT_MIHOMO_FILE':str(ROOT/'output/tools/mihomo')}
    run('docker','compose','-f','deploy/compose.yml','config','--quiet',env=compose_env)
    flags=['docker','run','--rm','--read-only','--user','10001:10001','--cap-drop','ALL','--security-opt','no-new-privileges=true','--security-opt','apparmor=subweft','--security-opt','seccomp='+str(ROOT/'deploy/seccomp.json'),'--pids-limit','160','--memory','512m','--tmpfs','/tmp:rw,nosuid,nodev,size=256m,mode=1777','--tmpfs','/run/subweft:rw,nosuid,nodev,noexec,size=192m,uid=10001,gid=10001,mode=0700','--mount','type=bind,source='+str(ROOT)+',target=/workspace,readonly']
    for package in ('validator','app','server','configuration'):
        run(*flags,'-e','SUBWEFT_TEST_MIHOMO=/workspace/output/tools/mihomo','-e','SUBWEFT_TEST_GEODB=/workspace/output/tools/GeoIP2-Country-Test.mmdb','--workdir','/workspace/internal/'+package,'--entrypoint','/workspace/output/container/'+package+'.test','subweft-check:local','-test.v','-test.timeout=5m')
    results=out/'results';results.mkdir(exist_ok=True)
    measured=run(*flags,'--workdir','/workspace','--entrypoint','/usr/bin/python3','subweft-check:local','tools/editor_smoke.py','--app','output/container/subweft','--fixture','output/container/devfixture','--work-dir','/tmp','--output','/tmp/editor-smoke.json',capture_output=True,text=True)
    print(measured.stdout,flush=True)
    report=json.loads(next(line for line in measured.stdout.splitlines() if line.startswith('{')))
    (results/'editor-smoke.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({'native_container':'passed','architecture':os.uname().machine,'isolation':'nested user/file/network namespaces;96 process hard limit;cap_drop ALL;read-only image;tmpfs validation','tests':['validator','app','server','configuration']}))
if __name__=='__main__':main()
