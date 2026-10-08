#!/usr/bin/env python3
"""Synthetic migration baseline; production sources are never edited.

Run from repository root: python3 cache/migration-performance-20261008/measure.py
Outputs and instrumented source copy live in ignored .tmp/migration-performance.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import signal
import sqlite3
import subprocess
import time

REPO = Path.cwd().resolve()
WORK = REPO / '.tmp/migration-performance'
OUT = WORK / 'results.json'
CASES = [
    # name, groups, files/group, messages/group, text bytes, distinct cwd, ignored bytes/line
    ('base', 16, 1, 64, 512, 1, 0),
    ('bytes4', 16, 1, 64, 2048, 1, 0),
    ('files4', 16, 4, 64, 512, 1, 0),
    ('groups4', 64, 1, 16, 512, 1, 0),
    ('groups16', 256, 1, 4, 512, 1, 0),
    ('cwd16', 16, 1, 64, 512, 16, 0),
    ('one_group', 1, 16, 1024, 512, 1, 0),
    ('total4', 64, 1, 64, 512, 1, 0),
    ('total16', 256, 1, 64, 512, 1, 0),
    ('ignored_large', 16, 1, 64, 512, 1, 16384),
    ('body64MiB', 16, 1, 64, 65536, 1, 0),
    ('no_git_256', 256, 1, 4, 512, 0, 0),
    ('no_git_1024', 1024, 1, 1, 512, 0, 0),
]


def command(args, cwd=REPO):
    subprocess.run(args, cwd=cwd, check=True)


def replace(path, old, new, count=1):
    text = path.read_text()
    assert text.count(old) == count, (path, old, text.count(old))
    path.write_text(text.replace(old, new))


def instrument(copy):
    probe = copy / 'internal/perfprobe'
    probe.mkdir()
    (probe / 'probe.go').write_text('''package perfprobe
import ("encoding/json"; "fmt"; "os"; "runtime"; "time")
var Counts = map[string]int64{}
var Seconds = map[string]float64{}
func Start(name string) func() { t := time.Now(); return func(){Seconds[name] += time.Since(t).Seconds()} }
func Emit(){ var m runtime.MemStats; runtime.ReadMemStats(&m); b,_ := json.Marshal(map[string]any{"counts":Counts,"seconds":Seconds,"total_alloc_bytes":m.TotalAlloc,"heap_inuse_bytes":m.HeapInuse}); fmt.Fprintln(os.Stderr,"PERFPROBE "+string(b)) }
''')
    for file in ['core/migrate.go', 'core/repo_path.go', 'ingest/codex/adapter.go']:
        path = copy / 'internal' / file
        replace(path, 'import (', 'import (\n"github.com/ryotapoi/somniloq/internal/perfprobe"')
    path = copy / 'internal/core/migrate.go'
    replace(path, '\tdb, digest, copied, err := prepareMigration', '\tdefer perfprobe.Emit()\n\tendPrepare := perfprobe.Start("prepare")\n\tdb, digest, copied, err := prepareMigration')
    replace(path, '\tif err != nil {\n\t\treturn nil, err\n\t}', '\tendPrepare()\n\tif err != nil {\n\t\treturn nil, err\n\t}')
    replace(path, '\t\tgroups, readErrors := adapter.BuildMigrationGroups', '\t\tendBuild := perfprobe.Start("build")\n\t\tgroups, readErrors := adapter.BuildMigrationGroups')
    replace(path, '\t\tscan := migrationInput', '\t\tendBuild()\n\t\tscan := migrationInput')
    replace(path, '\tevidence := map[string][]migrationEvidence{}', '\tendEvidence := perfprobe.Start("evidence")\n\tevidence := map[string][]migrationEvidence{}')
    replace(path, '\tsuccessful := map[string]map[int]bool{}', '\tendEvidence()\n\tendReplace := perfprobe.Start("replace")\n\tsuccessful := map[string]map[int]bool{}')
    replace(path, '\t// Same-named old history', '\tendReplace()\n\t// Same-named old history')
    replace(path, 'func checkMigrationSnapshot(adapter codex.Adapter, scan migrationInput, group codex.Group) error {', 'func checkMigrationSnapshot(adapter codex.Adapter, scan migrationInput, group codex.Group) error {\n\tdefer perfprobe.Start("snapshot")()\n\tperfprobe.Counts["snapshot_checks"]++')
    replace(path, '\t\terr = tx.QueryRow(`SELECT legacy_rowid', '\t\tperfprobe.Counts["old_lookup_sql"]++\n\t\tendLookup := perfprobe.Start("old_lookup")\n\t\terr = tx.QueryRow(`SELECT legacy_rowid')
    replace(path, '\t\tif err == sql.ErrNoRows {', '\t\tendLookup()\n\t\tif err == sql.ErrNoRows {')
    path = copy / 'internal/core/repo_path.go'
    replace(path, '\tout, err := cmd.Output()', '\tperfprobe.Counts["git"]++\n\tendGit := perfprobe.Start("git")\n\tout, err := cmd.Output()\n\tendGit()')
    replace(path, '\tout, err = cmd.Output()', '\tperfprobe.Counts["git"]++\n\tendGit = perfprobe.Start("git")\n\tout, err = cmd.Output()\n\tendGit()', 2)
    path = copy / 'internal/ingest/codex/adapter.go'
    replace(path, '\treturn ingest.ScanFilesRecursive(rootDir, func(path string) bool {\n\t\treturn strings.HasSuffix(path, ".jsonl")\n\t})', '\tdefer perfprobe.Start("scan")()\n\tperfprobe.Counts["scan"]++\n\tfiles, errs := ingest.ScanFilesRecursive(rootDir, func(path string) bool {\n\t\treturn strings.HasSuffix(path, ".jsonl")\n\t})\n\tperfprobe.Counts["scan_file_results"] += int64(len(files))\n\treturn files, errs')
    command(['gofmt', '-w', str(probe), str(copy/'internal/core/migrate.go'), str(copy/'internal/core/repo_path.go'), str(path)])


def setup(case):
    name, groups, parts, messages, size, cwd_count, ignored = case
    base = WORK / name
    base.mkdir()
    root = base / 'logs'
    root.mkdir()
    repos = []
    for n in range(cwd_count):
        repo = base / f'repo{n}'
        repo.mkdir()
        subprocess.run(['git', 'init', '-q', str(repo)], check=True)
        repos.append(str(repo))
    schema = re.search(r'const legacySnapshotSchema = `(.*?)`', (REPO/'internal/core/migrate_snapshot.go').read_text(), re.S)[1]
    db = sqlite3.connect(base/'old.db')
    db.executescript(schema)
    total = lines = max_group = 0
    for g in range(groups):
        sid = f'session-{g}'
        db.execute("INSERT INTO sessions(source,session_id,imported_at) VALUES('codex',?,'2026-01-01')", (sid,))
        group_bytes = 0
        for p in range(parts):
            path = root / f'{g:04d}-{p:04d}.jsonl'
            records = [{'type':'session_meta', 'payload':{'id':sid, 'cwd':repos[g % cwd_count] if cwd_count else ''}}]
            for m in range(p*messages//parts, (p+1)*messages//parts):
                text = 'x' * size
                records.append({'type':'response_item', 'timestamp':'2026-01-01T00:00:00Z', 'payload':{'type':'message', 'role':'assistant', 'id':f'm{g}-{m}', 'content':[{'type':'output_text', 'text':text}]}})
                line = len(records)
                if m % 4 == 0:
                    uuid = 'codex:' + hashlib.sha256(f'{path}\0{line}'.encode()).hexdigest()
                    db.execute("INSERT INTO messages(uuid,source,session_id,role,content,timestamp) VALUES(?,'codex',?,'assistant',?,'2026-01-01')", (uuid,sid,text))
                if ignored:
                    records.append({'type':'event_msg', 'payload':{'type':'token_count','padding':'i'*ignored}})
            data = ''.join(json.dumps(r, separators=(',',':'))+'\n' for r in records).encode()
            path.write_bytes(data)
            group_bytes += len(data)
            total += len(data)
            lines += len(records)
        max_group = max(max_group, group_bytes)
    # Keep one unrelated legacy conversation to exercise retention.
    db.execute("INSERT INTO sessions(source,session_id,imported_at) VALUES('claude_code','retained','2026-01-01')")
    db.execute("INSERT INTO messages(uuid,source,session_id,role,content,timestamp) VALUES('retained','claude_code','retained','user','keep','')")
    db.commit()
    db.close()
    (base/'config.toml').write_text(f'db = "{base}/new.db"\n[[inputs]]\nsource = "codex"\nroot = "{root}"\n')
    return base, dict(name=name, groups=groups, files=groups*parts, messages=groups*messages, lines=lines, bytes=total, max_group_bytes=max_group, cwd=cwd_count, old_messages=groups*((messages+3)//4)+1, snapshot_bytes=(base/'old.db').stat().st_size)


def run(binary, base, spec, retry, measured):
    before = hashlib.sha256((base/'old.db').read_bytes()).hexdigest()
    args = ['/usr/bin/time', '-l', str(binary), 'migrate', '--config', str(base/'config.toml'), '--from', str(base/'old.db')]
    start = time.monotonic()
    with (base/'stdout').open('w') as out, (base/'stderr').open('w') as err:
        proc = subprocess.Popen(args, stdout=out, stderr=err, start_new_session=True)
        reason = None
        while proc.poll() is None:
            if time.monotonic()-start > 60:
                reason = 'time limit'
            # time(1) has a single CLI child; monitor its RSS every 250 ms.
            child_query = subprocess.run(['pgrep','-P',str(proc.pid)], capture_output=True, text=True)
            if child_query.returncode not in (0, 1):
                os.killpg(proc.pid, signal.SIGTERM)
                proc.wait()
                raise RuntimeError(child_query.stderr)
            children = child_query.stdout.split()
            for pid in children:
                rss = subprocess.run(['ps','-o','rss=','-p',pid], capture_output=True, text=True).stdout.strip()
                if rss and int(rss) > 1024*1024:
                    reason = 'RSS limit'
            if reason:
                os.killpg(proc.pid, signal.SIGTERM)
                proc.wait()
                raise RuntimeError(f'{spec["name"]}: {reason}')
            time.sleep(.25)
    stderr = (base/'stderr').read_text()
    assert proc.returncode == 0, stderr
    summary = json.loads((base/'stdout').read_text())
    assert summary['groups_replaced'] == spec['groups'] and summary['groups_failed'] == 0
    assert summary['copy_performed'] == (not retry)
    assert summary['legacy_messages_retained'] == 1 and summary['legacy_replacement_failures'] == 0
    assert summary['legacy_messages_removed'] == (0 if retry else spec['old_messages']-1)
    db = sqlite3.connect(base/'new.db')
    assert db.execute('SELECT count(*) FROM messages').fetchone()[0] == spec['messages']
    assert db.execute('SELECT count(*) FROM sessions').fetchone()[0] == spec['groups']
    text_size = next(c[4] for c in CASES if c[0] == spec['name'])
    assert db.execute('SELECT min(length(content)),max(length(content)),sum(length(content)) FROM messages').fetchone() == (text_size, text_size, text_size*spec['messages'])
    assert db.execute("SELECT count(*) FROM messages WHERE role<>'assistant' OR membership<>'body' OR timestamp<>'2026-01-01T00:00:00Z'").fetchone()[0] == 0
    assert db.execute('SELECT count(*) FROM import_state WHERE last_offset=file_size').fetchone()[0] == spec['files']
    assert db.execute('PRAGMA integrity_check').fetchone()[0] == 'ok'
    db.close()
    assert hashlib.sha256((base/'old.db').read_bytes()).hexdigest() == before
    assert not any(Path(str(base/'old.db')+suffix).exists() for suffix in ['-wal','-shm','-journal'])
    wall, user, system = map(float, re.search(r'([\d.]+) real\s+([\d.]+) user\s+([\d.]+) sys', stderr).groups())
    rss = int(re.search(r'(\d+)\s+maximum resident set size', stderr)[1])
    result = dict(retry=retry, instrumented=measured, wall_s=wall, user_s=user, system_s=system, peak_rss_bytes=rss, summary=summary)
    if measured:
        result['probe'] = json.loads(re.search(r'PERFPROBE (.+)',stderr)[1])
    return result


def main():
    assert not WORK.exists(), f'Remove only this owned scratch directory before rerunning: {WORK}'
    WORK.mkdir(parents=True)
    os.environ['GOCACHE'] = str(WORK/'go-cache')
    os.environ['GOFLAGS'] = '-buildvcs=false'
    copy = WORK/'source'
    copy.mkdir()
    for name in ['go.mod','go.sum','internal','cmd']:
        src = REPO/name
        if src.is_dir():
            shutil.copytree(src, copy/name)
        else:
            shutil.copy(src, copy/name)
    command(['go','build','-o',str(WORK/'plain'),'./cmd/somniloq'])
    instrument(copy)
    command(['go','build','-o',str(WORK/'probe'),'./cmd/somniloq'], cwd=copy)
    report = dict(commit=subprocess.check_output(['git','rev-parse','HEAD'], text=True).strip(), go=subprocess.check_output(['go','version'], text=True).strip(), os=subprocess.check_output(['sw_vers'], text=True).strip(), cases=[])
    for case in CASES:
        base, spec = setup(case)
        results = []
        for measured in [False, False, False, True]:
            for path in base.glob('new.db*'):
                path.unlink()
            results.append(run(WORK/('probe' if measured else 'plain'),base,spec,False,measured))
            results.append(run(WORK/('probe' if measured else 'plain'),base,spec,True,measured))
        report['cases'].append(dict(spec=spec, runs=results))
        OUT.write_text(json.dumps(report, indent=2)+'\n')
        print(json.dumps(dict(spec=spec, runs=[{k:v for k,v in r.items() if k not in ['summary']} for r in results])), flush=True)
        if sum(p.stat().st_size for p in WORK.rglob('*') if p.is_file()) > 2*1024**3:
            raise RuntimeError('disk budget exceeded')
    print(f'Results: {OUT}', flush=True)


if __name__ == '__main__':
    main()
