#!/usr/bin/env python3
"""Build application-only candidate archives from clean committed source."""
from pathlib import Path
import argparse
import gzip
import hashlib
import io
import json
import os
import platform
import subprocess
import tarfile

ROOT=Path(__file__).resolve().parents[1]
def git(*args):return subprocess.check_output(['git',*args],cwd=ROOT,text=True).strip()
def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--platform',choices=['all','native'],default='all')
    args=parser.parse_args()
    if git('status','--porcelain'):raise SystemExit('Commit the intended source before packaging')
    version=(ROOT/'internal/buildinfo/VERSION').read_text().strip()
    commit=git('rev-parse','HEAD');stamp=int(git('show','-s','--format=%ct','HEAD'))
    node=Path(subprocess.check_output(['node','-p','process.execPath'],text=True).strip())
    candidates=[node.parent/'node_modules/npm/bin/npm-cli.js',node.parent.parent/'lib/node_modules/npm/bin/npm-cli.js']
    npm=next((p for p in candidates if p.is_file()),None)
    if npm is None:raise SystemExit('Cannot find the installed npm CLI')
    for command in ('ci','run'):
        args=['ci','--ignore-scripts','--no-audit','--no-fund'] if command=='ci' else ['run','build']
        subprocess.run([str(node),str(npm),*args],cwd=ROOT/'web',check=True)
    output=ROOT/'output/packages'/commit;output.mkdir(parents=True,exist_ok=True)
    common=['LICENSE','THIRD_PARTY.md','deploy/Dockerfile','deploy/compose.yml','deploy/seccomp.json','deploy/subweft.apparmor','deploy/runtime.env.example','deploy/nginx.conf.example','internal/validator/lock.json','tools/fetch_validator.py']
    sums=[]
    platforms=[('linux','amd64'),('linux','arm64'),('windows','amd64')]
    if args.platform=='native':
        native=({'Linux':'linux','Windows':'windows'}.get(platform.system()),{'x86_64':'amd64','AMD64':'amd64','aarch64':'arm64'}.get(platform.machine()))
        if native not in platforms:raise SystemExit('Unsupported native platform')
        platforms=[native]
    for system,arch in platforms:
        name=f'subweft-v{version}-{system}-{arch}'
        target=output/(name+'.tar.gz')
        if target.exists():raise SystemExit('Refusing to overwrite an existing candidate archive')
        binary=output/('subweft.exe' if system=='windows' else 'subweft')
        subprocess.run(['go','build','-trimpath','-buildvcs=true','-tags','webui','-o',str(binary),'./cmd/subweft'],cwd=ROOT,env={**os.environ,'CGO_ENABLED':'0','GOOS':system,'GOARCH':arch},check=True)
        files={f:(ROOT/f).read_bytes() for f in common}
        # Preserve upstream license/notice files in the actual binary distribution.
        modules=json.loads('['+subprocess.check_output(['go','list','-m','-json','all'],cwd=ROOT,text=True).replace('}\n{','},\n{')+']')
        def notices(directory,prefix):
            for file in directory.rglob('*'):
                if not file.is_file() or any(p in ('node_modules','.git','testdata') for p in file.relative_to(directory).parts):continue
                if file.name.upper().startswith(('LICENSE','COPYING','COPYRIGHT','NOTICE')):
                    files['licenses/'+prefix+'/'+file.relative_to(directory).as_posix()]=file.read_bytes()
        for module in modules:
            if module.get('Main') or not module.get('Dir'):continue
            notices(Path(module['Dir']),'go/'+module['Path']+'@'+module['Version'])
        goroot=Path(subprocess.check_output(['go','env','GOROOT'],text=True).strip())
        files['licenses/Go-LICENSE']=(goroot/'LICENSE').read_bytes()
        lock=json.loads((ROOT/'web/package-lock.json').read_text())
        for path,package in lock['packages'].items():
            if not path or package.get('dev'):continue
            notices(ROOT/'web'/path,'npm/'+path.removeprefix('node_modules/')+'@'+package['version'])
        files[binary.name]=binary.read_bytes()
        files['INSTALL.txt']=(f'Subweft {version}\nSource commit: {commit}\nPlatform: {system}/{arch}\n\nApplication-only candidate. Mihomo is NOT bundled.\nInstallation and human acceptance:\nhttps://github.com/fjzhangZzzzzz/subweft/blob/{commit}/docs/human-acceptance.md\n\nLinux container instructions:\nhttps://github.com/fjzhangZzzzzz/subweft/blob/{commit}/docs/container.md\n\nVerify SHA256SUMS before use. Preserve the master key separately from SQLite.\n').encode()
        files['BUILD.json']=(json.dumps({'version':version,'commit':commit,'dirty':False,'platform':system+'/'+arch,'files':{f:hashlib.sha256(b).hexdigest() for f,b in sorted(files.items())}},indent=2)+'\n').encode()
        buffer=io.BytesIO()
        with tarfile.open(fileobj=buffer,mode='w') as tar:
            for f,b in sorted(files.items()):
                info=tarfile.TarInfo(name+'/'+f);info.size=len(b);info.mtime=stamp;info.mode=0o755 if f==binary.name else 0o644
                tar.addfile(info,io.BytesIO(b))
        payload=gzip.compress(buffer.getvalue(),mtime=0)
        with target.open('xb') as handle:handle.write(payload)
        binary.unlink();sums.append(hashlib.sha256(payload).hexdigest()+'  '+target.name)
    (output/'SHA256SUMS').write_text('\n'.join(sums)+'\n')
    print(json.dumps({'version':version,'commit':commit,'platforms':['/'.join(p) for p in platforms],'directory':str(output),'contains_mihomo':False}))
if __name__=='__main__':main()
