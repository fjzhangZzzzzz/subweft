#!/usr/bin/env python3
"""Exercise the exact packaged Windows development binary with synthetic data."""
from pathlib import Path
import hashlib
import json
import os
import subprocess
import sys
import tarfile

ROOT=Path(__file__).resolve().parents[1]
def main():
    if os.name!='nt':raise SystemExit('Windows development verification only')
    commit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()
    packages=list((ROOT/'output/packages'/commit).glob('subweft-*-windows-amd64.tar.gz'))
    if len(packages)!=1:raise SystemExit('One exact Windows candidate is required')
    out=ROOT/'output/container';out.mkdir(parents=True,exist_ok=True)
    with tarfile.open(packages[0]) as archive:
        prefix=packages[0].name.removesuffix('.tar.gz')
        binary=archive.extractfile(prefix+'/subweft.exe').read()
        manifest=json.load(archive.extractfile(prefix+'/BUILD.json'))
        if hashlib.sha256(binary).hexdigest()!=manifest['files']['subweft.exe']:raise SystemExit('Candidate digest mismatch')
        (out/'subweft.exe').write_bytes(binary)
    subprocess.run(['go','build','-o',str(out/'devfixture.exe'),'./tools/devfixture'],cwd=ROOT,check=True)
    subprocess.run([sys.executable,'tools/benchmark.py','--app',str(out/'subweft.exe'),'--fixture',str(out/'devfixture.exe'),'--output',str(out/'results/benchmark.json')],cwd=ROOT,check=True)
if __name__=='__main__':main()
