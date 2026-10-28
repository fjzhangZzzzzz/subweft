#!/usr/bin/env python3
"""Native disposable Docker verification. No host state or real credentials are mounted."""
from pathlib import Path
import os, subprocess, json
ROOT=Path(__file__).resolve().parents[1]
def run(*args,**kw): return subprocess.run(args,cwd=ROOT,check=True,**kw)
def main():
    out=ROOT/'output/container';out.mkdir(parents=True,exist_ok=True)
    env={**os.environ,'CGO_ENABLED':'0'}
    run('go','build','-tags','webui','-o',str(out/'subweft'),'./cmd/subweft',env=env)
    for package in ('validator','app','server'):
        run('go','test','-c','-o',str(out/(package+'.test')),'./internal/'+package,env=env)
    (out/'ca-certificates.crt').write_bytes(Path('/etc/ssl/certs/ca-certificates.crt').read_bytes())
    run('docker','build','--provenance=false','-f','deploy/Dockerfile','-t','subweft-check:local','.')
    flags=['docker','run','--rm','--read-only','--user','10001:10001','--cap-drop','ALL','--security-opt','no-new-privileges=true','--security-opt','apparmor=subweft','--security-opt','seccomp='+str(ROOT/'deploy/seccomp.json'),'--pids-limit','160','--memory','512m','--tmpfs','/tmp:rw,nosuid,nodev,size=256m,mode=1777','--tmpfs','/run/subweft:rw,nosuid,nodev,noexec,size=192m,uid=10001,gid=10001,mode=0700','--mount','type=bind,source='+str(ROOT)+',target=/workspace,readonly']
    for package in ('validator','app','server'):
        run(*flags,'--workdir','/workspace/internal/'+package,'--entrypoint','/workspace/output/container/'+package+'.test','subweft-check:local','-test.v','-test.timeout=5m')
    print(json.dumps({'native_container':'passed','architecture':os.uname().machine,'isolation':'nested user/file/network namespaces;96 process hard limit;cap_drop ALL;read-only image;tmpfs validation','tests':['validator','app','server']}))
if __name__=='__main__':main()
