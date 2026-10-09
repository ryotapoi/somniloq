#!/usr/bin/env python3
"""Paired synthetic migration comparison; all databases live in ignored .tmp."""
import importlib.util
import json
import os
import re
import sqlite3
import hashlib
from pathlib import Path
import shutil
import subprocess
import sys
import time
sys.dont_write_bytecode = True
REPO = Path.cwd().resolve()
WORK = Path(os.environ.get('SOMNILOQ_PERF_WORK', REPO / '.tmp/migration-metadata-measure')).resolve()
BASELINE = os.environ.get('SOMNILOQ_PERF_BASELINE', 'HEAD')
spec = importlib.util.spec_from_file_location('fixture', REPO / 'cache/migration-performance-20261008/measure.py')
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
spec = importlib.util.spec_from_file_location('digest', REPO / 'cache/migration-refresh-performance-20261009/measure.py')
d = importlib.util.module_from_spec(spec); spec.loader.exec_module(d)
m.WORK = WORK / 'inputs'
m.CASES = [('base', 64, 1, 64, 512, 0, 0), ('groups4', 256, 1, 64, 512, 0, 0), ('body64MiB', 16, 1, 64, 65536, 0, 0)]
def edit(path, old, new):
    s=path.read_text(); assert old in s, (path,old); path.write_text(s.replace(old,new))
def instrument(source, candidate):
    probe=source/'internal/perfprobe'; probe.mkdir()
    (probe/'probe.go').write_text('''package perfprobe
import("encoding/json";"fmt";"os";"io";"runtime";"time")
var Counts=map[string]int64{}
var Seconds=map[string]float64{}
func Start(n string) func(){t:=time.Now();return func(){Seconds[n]+=time.Since(t).Seconds()}}
func ReadFile(p string)([]byte,error){Counts["full_reads"]++;b,e:=os.ReadFile(p);Counts["full_bytes"]+=int64(len(b));return b,e}
type Reader struct{io.Reader; Name string}
func(r Reader)Read(b []byte)(int,error){n,e:=r.Reader.Read(b);Counts[r.Name+"_bytes"]+=int64(n);Counts[r.Name+"_calls"]++;return n,e}
func Emit(){var m runtime.MemStats;runtime.ReadMemStats(&m);b,_:=json.Marshal(map[string]any{"counts":Counts,"seconds":Seconds,"total_alloc_bytes":m.TotalAlloc});fmt.Fprintln(os.Stderr,"PERFPROBE "+string(b))}
''')
    core=source/'internal/core/migrate.go'; group=source/'internal/ingest/codex/group.go'
    for p in [core,group]: edit(p,'import (','import (\n"github.com/ryotapoi/somniloq/internal/perfprobe"')
    edit(core,'db, digest, copied, err := prepareMigration','defer perfprobe.Emit()\n db, digest, copied, err := prepareMigration')
    edit(group,'func (a Adapter) BuildMigrationIndex(root string, paths []string, importedAt string) ([]Group, []error) {','func (a Adapter) BuildMigrationIndex(root string, paths []string, importedAt string) ([]Group, []error) {\n defer perfprobe.Start("index")()')
    edit(group,'func (a Adapter) BuildMigrationGroups(root string, paths []string, importedAt string) ([]Group, []error) {','func (a Adapter) BuildMigrationGroups(root string, paths []string, importedAt string) ([]Group, []error) {\n defer perfprobe.Start("body")()')
    edit(core,'func checkMigrationGroup(group codex.Group) error {','func checkMigrationGroup(group codex.Group) error {\n defer perfprobe.Start("check")()')
    edit(core,'func replaceMigrationGroup(db *DB, input Input, g codex.Group, digest, importedAt string) (int, error) {','func replaceMigrationGroup(db *DB, input Input, g codex.Group, digest, importedAt string) (int, error) {\n defer perfprobe.Start("save_including_check")()')
    if candidate:
        edit(group,'bufio.NewReader(file)','bufio.NewReader(perfprobe.Reader{Reader:file,Name:"metadata"})')
        edit(group,'_, err = buffer.ReadFrom(file)','perfprobe.Counts["full_reads"]++\n _, err = buffer.ReadFrom(perfprobe.Reader{Reader:file,Name:"full"})')
    else:
        edit(group,'os.ReadFile(path)','perfprobe.ReadFile(path)')
        edit(group,'\"os\"','')
        edit(core,'_, err = io.CopyBuffer(hash, struct{ io.Reader }{file}, buf)','perfprobe.Counts["hash_reads"]++\n var n int64\n n, err = io.CopyBuffer(hash, struct{ io.Reader }{file}, buf)\n perfprobe.Counts["hash_bytes"]+=n')
    subprocess.run(['gofmt','-w',str(probe),str(core),str(group)],check=True)
def run(binary, base, details, retry, measured):
    before=hashlib.sha256((base/'old.db').read_bytes()).hexdigest()
    proc=subprocess.run(['/usr/bin/time','-l',str(binary),'migrate','--config',str(base/'config.toml'),'--from',str(base/'old.db')],capture_output=True,text=True,timeout=60)
    assert proc.returncode==0,proc.stderr
    summary=json.loads(proc.stdout)
    assert summary['groups_replaced']==details['groups'] and summary['groups_failed']==0
    assert summary['copy_performed']==(not retry) and summary['legacy_messages_retained']==1
    assert summary['legacy_messages_removed']==(0 if retry else details['old_messages']-1)
    with sqlite3.connect(base/'new.db') as db:
        assert db.execute('SELECT count(*) FROM messages').fetchone()[0]==details['messages']
        assert db.execute('SELECT count(*) FROM import_state WHERE last_offset=file_size').fetchone()[0]==details['files']
        assert db.execute('PRAGMA integrity_check').fetchone()[0]=='ok'
    assert hashlib.sha256((base/'old.db').read_bytes()).hexdigest()==before
    assert not any(Path(str(base/'old.db')+s).exists() for s in ['-wal','-shm','-journal'])
    wall,user,system=map(float,re.search(r'([\d.]+) real\s+([\d.]+) user\s+([\d.]+) sys',proc.stderr).groups())
    rss=int(re.search(r'(\d+)\s+maximum resident set size',proc.stderr)[1])
    assert rss<1024**3,'RSS budget exceeded'
    result=dict(wall_s=wall,user_s=user,system_s=system,peak_rss_bytes=rss,retry=retry,instrumented=measured,summary=summary)
    if measured:result['probe']=json.loads(re.search(r'PERFPROBE (.+)',proc.stderr)[1])
    return result

def main():
    assert not WORK.exists(), 'Use a new scratch directory'
    WORK.mkdir(parents=True);m.WORK.mkdir()
    os.environ['GOCACHE']=str(REPO/'.tmp/go-cache');os.environ['GOFLAGS']='-buildvcs=false'
    report={'baseline':subprocess.check_output(['git','rev-parse',BASELINE],text=True).strip(),'candidate':'working tree','gc_environment':{k:os.environ.get(k) for k in ['GOGC','GOMEMLIMIT','GOMAXPROCS']},'go':subprocess.check_output(['go','version'],text=True).strip(),'os':subprocess.check_output(['sw_vers'],text=True).strip(),'cases':[]}
    for version in ['baseline','candidate']:
        source=WORK/(version+'-source');source.mkdir()
        if version=='baseline':
            archive=subprocess.check_output(['git','archive',BASELINE]);subprocess.run(['tar','-x','-C',str(source)],input=archive,check=True)
        else:
            for name in ['go.mod','go.sum','internal','cmd']:
                src=REPO/name
                if src.is_dir():shutil.copytree(src,source/name)
                else:shutil.copy(src,source/name)
        subprocess.run(['go','build','-o',str(WORK/version),'./cmd/somniloq'],cwd=source,check=True)
        instrument(source,version=='candidate')
        subprocess.run(['go','build','-o',str(WORK/(version+'-probe')),'./cmd/somniloq'],cwd=source,check=True)
    started=time.monotonic()
    for case in m.CASES:
        base,details=m.setup(case);runs=[];expected={}
        for repetition in range(4):
            for version in (['baseline','candidate'] if repetition%2==0 else ['candidate','baseline']):
                for path in base.glob('new.db*'):path.unlink()
                for retry in [False,True]:
                    measured=repetition==3
                    r=run(WORK/(version+('-probe' if measured else '')),base,details,retry,measured)
                    rows=d.digest_tables(base/'new.db')
                    if retry in expected: assert rows==expected[retry],(case[0],version,'different persisted rows')
                    else: expected[retry]=rows
                    r.update(version=version,repetition=repetition,rows_equal=True)
                    runs.append(r)
                    print(case[0],version,repetition,'retry' if retry else 'first',r['wall_s'],flush=True)
        imports=[]
        for version in ['baseline','candidate']:
            cfg=base/'import.toml'; db=base/(version+'-import.db')
            cfg.write_text(f'db = "{db}"\n[[inputs]]\nsource = "codex"\nroot = "{base}/logs"\n')
            for phase in ['first','skip']:
                t=time.monotonic();r=subprocess.run([str(WORK/version),'import','--config',str(cfg)],capture_output=True,text=True,check=True)
                imports.append(dict(version=version,phase=phase,wall_s=time.monotonic()-t,stdout=r.stdout,stderr=r.stderr))
        report['cases'].append(dict(spec=details,runs=runs,imports=imports))
        (WORK/'results.json').write_text(json.dumps(report,indent=2)+'\n')
        assert time.monotonic()-started<900,'measurement time budget exceeded'
        assert sum(p.stat().st_size for p in WORK.rglob('*') if p.is_file())<2*1024**3,'disk budget exceeded'
if __name__=='__main__':main()
