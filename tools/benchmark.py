#!/usr/bin/env python3
"""Measure the real application process with isolated synthetic HTTP/OIDC data."""
from __future__ import annotations
import argparse
import concurrent.futures
import ctypes
import http.cookiejar
import json
import os
from pathlib import Path
import re
import socket
import shutil
import subprocess
import tempfile
import threading
import time
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[1]

def port():
    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
        return s.getsockname()[1]

def memory(pid):
    """RSS and OS peak for one owned PID, never the benchmark driver."""
    if os.name == 'nt':
        from ctypes import wintypes as w
        class Counters(ctypes.Structure):
            _fields_ = [('cb',w.DWORD),('PageFaultCount',w.DWORD)] + [(k,ctypes.c_size_t) for k in ('PeakWorkingSetSize','WorkingSetSize','QuotaPeakPagedPoolUsage','QuotaPagedPoolUsage','QuotaPeakNonPagedPoolUsage','QuotaNonPagedPoolUsage','PagefileUsage','PeakPagefileUsage')]
        k=ctypes.WinDLL('kernel32',use_last_error=True)
        k.OpenProcess.restype=w.HANDLE
        h=k.OpenProcess(0x1010,False,pid)
        if not h:return (0,0)
        c=Counters();c.cb=ctypes.sizeof(c)
        ps=ctypes.WinDLL('psapi');ps.GetProcessMemoryInfo.argtypes=[w.HANDLE,ctypes.c_void_p,w.DWORD];k.CloseHandle.argtypes=[w.HANDLE]
        ok=ps.GetProcessMemoryInfo(h,ctypes.byref(c),c.cb)
        k.CloseHandle(h)
        return (c.WorkingSetSize,c.PeakWorkingSetSize) if ok else (0,0)
    try:
        text=Path(f'/proc/{pid}/status').read_text()
        return tuple(int(re.search(r'^'+key+r':\s+(\d+)',text,re.M)[1])*1024 for key in ('VmRSS','VmHWM'))
    except (OSError,TypeError):return (0,0)

def child_core_peak(pid):
    # Linux validates inside namespace descendants; enumerate only this application's tree.
    if os.name=='nt':return 0
    try:
        pending=[pid];peak=0
        while pending:
            current=pending.pop()
            for task in Path(f'/proc/{current}/task').iterdir():
                try:pending.extend(map(int,(task/'children').read_text().split()))
                except OSError:pass
            try:
                if Path(f'/proc/{current}/exe').resolve().name in ('mihomo','validator'):
                    peak=max(peak,memory(current)[1])
            except OSError:pass
        return peak
    except OSError:return 0

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--app',type=Path)
    parser.add_argument('--fixture',type=Path)
    parser.add_argument('--requests',type=int,default=200)
    parser.add_argument('--work-dir',type=Path,default=ROOT/'output')
    parser.add_argument('--settle',type=int,default=30)
    parser.add_argument('--output',type=Path,default=ROOT/'output/benchmark.json')
    args=parser.parse_args()
    if not 20<=args.requests<=10000 or not 1<=args.settle<=120:raise SystemExit('invalid benchmark parameters')
    suffix='.exe' if os.name=='nt' else ''
    application=(args.app or ROOT/'output/bin'/('subweft'+suffix)).resolve()
    fixture=(args.fixture or ROOT/'output/bin'/('devfixture'+suffix)).resolve()
    core=(ROOT/'output/tools'/('mihomo'+suffix)).resolve()
    base='http://127.0.0.1:'+str(port());issuer='http://127.0.0.1:'+str(port())
    direct=urllib.request.build_opener(urllib.request.ProxyHandler({}))
    jar=http.cookiejar.CookieJar()
    client=urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPCookieProcessor(jar))
    csrf='';stage='initialization';children=[];stop=threading.Event();peaks={'app':0,'core':0}
    def call(path,body=None,method=None):
        req=urllib.request.Request(base+'/api/v1/'+path,data=None if body is None else json.dumps(body).encode(),method=method,headers={'Content-Type':'application/json','Origin':base,'X-CSRF-Token':csrf})
        with client.open(req,timeout=120) as response:return json.load(response)
    def ready(url,child):
        for _ in range(200):
            if child.poll() is not None:raise RuntimeError('process exited')
            try:
                with direct.open(url,timeout=.2) as r:
                    if r.status==200:return
            except OSError:pass
            time.sleep(.05)
        raise RuntimeError('readiness timeout')
    try:
        private=tempfile.mkdtemp(prefix='subweft-bench-',dir=args.work_dir)
        if True:
            root=Path(private);state=root/'state';key=root/'master.key'
            log=(root/'process.log').open('wb')
            flags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0
            env={**os.environ,'SUBWEFT_LISTEN':urllib.parse.urlsplit(base).netloc,'SUBWEFT_BASE_URL':base,'SUBWEFT_LOCAL_HTTP':'true','SUBWEFT_OIDC_ISSUER':issuer,'SUBWEFT_OIDC_CLIENT_ID':'subweft-local','SUBWEFT_OIDC_CLIENT_SECRET':'fixture-client-secret-not-for-production','SUBWEFT_ADMIN_SUBJECTS':'fixture-admin','SUBWEFT_MIHOMO_PATH':str(core),'SUBWEFT_FETCH_ALLOW_CIDRS':'127.0.0.1/32'}
            subprocess.run([str(application),'init','--data-dir',str(state),'--key-file',str(key)],cwd=ROOT,env=env,stdout=log,stderr=log,check=True,creationflags=flags)
            idp=subprocess.Popen([str(fixture),'--listen',urllib.parse.urlsplit(issuer).netloc,'--base-url',base],cwd=ROOT,env=env,stdout=log,stderr=log,creationflags=flags);children.append(idp)
            ready(issuer+'/.well-known/openid-configuration',idp)
            service=subprocess.Popen([str(application),'serve','--data-dir',str(state),'--key-file',str(key)],cwd=ROOT,env=env,stdout=log,stderr=log,creationflags=flags);children.append(service)
            def monitor():
                while not stop.wait(.01):
                    peaks['app']=max(peaks['app'],memory(service.pid)[1])
                    peaks['core']=max(peaks['core'],child_core_peak(service.pid))
            watcher=threading.Thread(target=monitor,daemon=True);watcher.start()
            ready(base+'/healthz',service)
            stage='test OIDC login'
            with client.open(base+'/auth/login') as response:query=urllib.parse.urlsplit(response.url).query;response.read()
            with client.open(urllib.request.Request(issuer+'/approve',data=query.encode())) as response:response.read()
            csrf=call('state')['csrf']
            stage='fetch ten HTTP sources'
            for i in range(10):
                src=call('sources',{'name':f'source-{i:02d}','kind':'http','url':issuer+f'/bench/provider/{i}','enabled':True,'tags':[],'interval':3600,'clientProvider':{'enabled':True,'interval':3600,'healthCheck':{'enable':False}}})
                call('sources/'+src['id']+'/refresh',{'revision':src['revision']})
            stage='fetch 300000-entry ruleset'
            resource=call('resources',{'name':'large-public-rules','kind':'http','type':'ruleset','behavior':'domain','format':'text','url':issuer+'/bench/rules.txt','enabled':True,'interval':3600})
            resource=call('resources/'+resource['id']+'/refresh',{'revision':resource['revision']})
            if resource['count']!=300000:raise RuntimeError('wrong rule count')
            stage='publish five profiles with native validation'
            links=[]
            for i in range(5):
                p=call('profiles',{'name':f'profile-{i:02d}','groups':[{'name':'Proxy','type':'select','allSources':True}],'rules':[{'type':'RULE-SET','value':resource['id'],'target':'Proxy','enabled':True},{'type':'MATCH','target':'Proxy','enabled':True}],'settings':{'mixedPort':7890}})
                call('profiles/'+p['id']+'/publish',{'revision':p['revision']})
                for j in range(4):
                    d=call('devices',{'name':f'device-{i}-{j}','profileId':p['id'],'state':'active'})
                    links.append(call('devices/'+d['id']+'/link')['url'])
            stage='warmup'
            for link in links:
                with direct.open(link) as response:response.read()
            time.sleep(args.settle)
            idle=memory(service.pid)[0]
            stage='concurrent cached downloads'
            def download(index):
                start=time.perf_counter()
                with direct.open(links[index%20],timeout=15) as response:body=response.read()
                return ((time.perf_counter()-start)*1000,len(body),body)
            with concurrent.futures.ThreadPoolExecutor(max_workers=10) as pool:results=list(pool.map(download,range(args.requests)))
            latency=sorted(r[0] for r in results)
            body=results[0][2].decode();resources=re.findall(r'https?://[^\s"\x27]+/resources/[^\s"\x27]+',body)
            stage='concurrent versioned resource downloads'
            def asset(index):
                with direct.open(resources[index%len(resources)],timeout=30) as response:return len(response.read())
            with concurrent.futures.ThreadPoolExecutor(max_workers=10) as pool:asset_bytes=list(pool.map(asset,range(20)))
            metrics=call('state');native_peak=metrics.get('validatorPeakRSSBytes',0);native_job_peak=metrics.get('validatorJobPeakRSSBytes',0)
            stage='actual CLI backup, restore and restart'
            request=urllib.request.Request(base+'/api/v1/backup',data=b'{}',headers={'Content-Type':'application/json','Origin':base,'X-CSRF-Token':csrf})
            with client.open(request,timeout=120) as response:(root/'backup.sqlite').write_bytes(response.read())
            service.terminate();service.wait(timeout=10)
            restored=root/'restored'
            subprocess.run([str(application),'restore','--backup-file',str(root/'backup.sqlite'),'--data-dir',str(restored),'--key-file',str(key)],cwd=ROOT,env=env,stdout=log,stderr=log,check=True,creationflags=flags)
            service=subprocess.Popen([str(application),'serve','--data-dir',str(restored),'--key-file',str(key)],cwd=ROOT,env=env,stdout=log,stderr=log,creationflags=flags);children.append(service)
            ready(base+'/healthz',service)
            for index,link in enumerate(links):
                with direct.open(link,timeout=15) as response:after=response.read()
                if after!=results[index%len(results)][2]:raise RuntimeError('restored config differs')
            for url in resources:
                with direct.open(url,timeout=30) as response:response.read()
            report={'scope':'real app executable; HTTP fetch+publish+immutable providers+300000 domain rules; loopback only','platform':os.name,'sources':10,'nodes':2000,'profiles':5,'devices':20,'rules':300000,'requests':args.requests,'concurrency':10,'failures':0,'response_bytes':results[0][1],'p50_ms':round(latency[len(latency)//2],2),'p95_ms':round(latency[(len(latency)-1)*95//100],2),'app_idle_rss_mib':round(idle/1048576,2),'app_peak_rss_mib':round(max(peaks['app'],memory(service.pid)[1])/1048576,2),'native_core_peak_rss_mib':(round(native_peak/1048576,2) if os.name!='nt' else None),'native_job_peak_rss_mib':(round(native_job_peak/1048576,2) if os.name!='nt' else None),'native_measurement':'Linux wait4 for core child and whole job separately; Windows unmeasured','sample_interval_ms':10,'settle_seconds':args.settle,'resource_downloads':len(asset_bytes),'resource_bytes':sum(asset_bytes),'container_total_peak_mib':None,'driver_excluded':True,'cli_restore_restart_passed':True}
            cgroup=Path('/sys/fs/cgroup/memory.peak')
            if cgroup.is_file():report['container_total_peak_mib']=round(int(cgroup.read_text())/1048576,2)
            report['targets_passed']=idle<=64*1048576 and report['p95_ms']<=300
            args.output.parent.mkdir(parents=True,exist_ok=True);args.output.write_text(json.dumps(report,indent=2)+'\n')
            print(json.dumps(report),flush=True)
            if os.name!='nt' and not report['native_core_peak_rss_mib']:raise RuntimeError('native RSS unmeasured')
            if not report['targets_passed']:raise RuntimeError('performance target not met')
    except Exception:
        print('benchmark_failed: '+stage,flush=True)
        raise SystemExit(1)
    finally:
        stop.set()
        for child in reversed(children):
            if child.poll() is None:
                child.terminate()
                try:child.wait(timeout=5)
                except subprocess.TimeoutExpired:child.kill();child.wait()
        if 'log' in locals():log.close()
        if 'root' in locals() and root.resolve().is_relative_to(args.work_dir.resolve()) and root.name.startswith('subweft-bench-'):shutil.rmtree(root)

if __name__=='__main__':main()
