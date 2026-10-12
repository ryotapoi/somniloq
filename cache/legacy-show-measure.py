import base64,hashlib,json,os,pathlib,subprocess,time
root=pathlib.Path('.tmp/legacy-show')
for side in ("before","after"):
 subprocess.run([str(root/side),"--help"],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,check=True,timeout=10)
for total in (1000,100000,"normal"):
 for refs in ((1,) if total=="normal" else (1,8)):
  selection=['slq1:legacy:'+('a'*64)+':codex:'+base64.urlsafe_b64encode(f's{i}'.encode()).decode().rstrip('=') for i in range(refs)]
  if total == 'normal': selection=[(root/'normal.ref').read_text().strip()]
  for case,flags in [('full',[]),('limit',['--limit','5']),('tail',['--tail','5'])]:
   expected=None
   for rnd in range(6):
    for side in (('before','after') if rnd%2==0 else ('after','before')):
     with open(root/'stdout.json','wb') as output,open(root/'stderr','wb') as err:
      start=time.perf_counter()
      p=subprocess.Popen([str(root/side),'show','--config',str(root/f'{total}.toml'),*selection,'--format','json',*flags],stdout=output,stderr=err)
      deadline=start+10
      while True:
       done,status,usage=os.wait4(p.pid,os.WNOHANG)
       if done: break
       if time.perf_counter()>deadline:
        p.kill();os.wait4(p.pid,0);raise TimeoutError()
       time.sleep(.001)
      elapsed=(time.perf_counter()-start)*1000
      p.returncode=os.waitstatus_to_exitcode(status)
     if p.returncode: raise RuntimeError((p.returncode,(root/'stderr').read_text()))
     assert not (root/'stderr').read_bytes()
     data=(root/'stdout.json').read_bytes();page=json.loads(data)
     digest=hashlib.sha256(data).hexdigest()
     if expected is None: expected=digest
     assert expected==digest,(total,refs,case,side)
     assert page['total']==refs*20
     assert page['count']==(refs*20 if case=='full' else 5)
     print(json.dumps(dict(kind='cli',total=total,refs=refs,case=case,round=rnd,side=side,ms=elapsed,cpu_ms=(usage.ru_utime+usage.ru_stime)*1000,rss_mib=usage.ru_maxrss/1048576,rows=page['count'],hash=digest)),flush=True)
