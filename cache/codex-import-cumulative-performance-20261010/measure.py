#!/usr/bin/env python3
"""Paired end-to-end Codex import measurements on isolated synthetic data."""
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import resource
import shutil
import signal
import sqlite3
import subprocess
import sys
import time

REPO = Path(__file__).resolve().parents[2]
WORK = REPO / '.tmp/codex-import-cumulative-performance-20261010'
OUT = Path(__file__).resolve().parent
BASELINE = 'cd4ac1316849fb44d640bc8abe6a9dbb6c7202c9'
CURRENT = '3dec7aa35a2884ca72bcf03547b650b688ba1554'
CASES = [('small', 16, 8, 65536), ('many_body', 64, 8, 65536), ('one_large_body', 1, 128, 262144)]
PHASES = ('initial', 'full', 'unchanged', 'append', 'same_size_edit', 'new_rollout')


def command(args, **kwargs):
    proc = subprocess.run(args, **kwargs)
    assert proc.returncode == 0, (args, proc.stdout, proc.stderr)
    return proc


def make_source(rev, label):
    dest = WORK / f'{label}-source'
    dest.mkdir()
    archive = subprocess.check_output(['git', 'archive', rev], cwd=REPO)
    command(['tar', '-x', '-C', str(dest)], input=archive)
    return dest


def build(source, label, probe=False):
    if probe:
        source = WORK / f'{label}-probe-source'
        if source.exists():
            shutil.rmtree(source)
        source.mkdir()
        original = WORK / f'{label}-source'
        for name in ('go.mod', 'go.sum'):
            shutil.copy(original / name, source / name)
        for name in ('cmd', 'internal'):
            shutil.copytree(original / name, source / name)
        package = source / 'internal/perfprobe'
        package.mkdir()
        (package / 'probe.go').write_text('''package perfprobe
import ("os"; "sync/atomic")
var Bytes int64
type File struct {file *os.File}
func Open(path string) (*File,error) {f,e:=os.Open(path); if e!=nil{return nil,e}; return &File{f},nil}
func (f *File) Read(p []byte) (int,error) {n,e:=f.file.Read(p); atomic.AddInt64(&Bytes,int64(n)); return n,e}
func (f *File) Close() error {return f.file.Close()}
func (f *File) Stat() (os.FileInfo,error) {return f.file.Stat()}
func ReadFile(path string) ([]byte,error) {b,e:=os.ReadFile(path); atomic.AddInt64(&Bytes,int64(len(b))); return b,e}
''')
        for relative in ('internal/ingest/codex/group.go', 'internal/ingest/codex/adapter.go'):
            file = source / relative
            content = file.read_text().replace('import (', 'import (\n"github.com/ryotapoi/somniloq/internal/perfprobe"', 1)
            content = content.replace('os.Open(', 'perfprobe.Open(').replace('os.ReadFile(', 'perfprobe.ReadFile(')
            if relative.endswith('adapter.go'):
                content = content.replace('\n\t"os"', '')
            file.write_text(content)
        main = source / 'cmd/somniloq/main.go'
        content = main.read_text().replace('import (', 'import (\n"syscall"\n"github.com/ryotapoi/somniloq/internal/perfprobe"', 1)
        content = content.replace('os.Exit(code)', 'var u syscall.Rusage\n syscall.Getrusage(syscall.RUSAGE_SELF, &u)\n fmt.Fprintf(os.Stderr, "PERF_RSS %d\\nPERF_BYTES %d\\n", u.Maxrss, perfprobe.Bytes)\n os.Exit(code)', 1)
        main.write_text(content)
        command(['gofmt', '-w', str(package / 'probe.go'), str(source / 'internal/ingest/codex/group.go'),
                 str(source / 'internal/ingest/codex/adapter.go'), str(main)], capture_output=True, text=True)
    binary = WORK / (label + ('-probe' if probe else ''))
    command(['go', 'build', '-o', str(binary), './cmd/somniloq'], cwd=source, capture_output=True, text=True, timeout=120)
    return binary


def line(owner, n, size):
    item = {'type': 'response_item', 'timestamp': '2026-01-01T00:00:00Z',
            'payload': {'type': 'message', 'role': 'user', 'id': f'{owner}-{n}',
                        'content': [{'type': 'input_text', 'text': f'{owner}-{n}-' + 'x' * size}]}}
    return (json.dumps(item, separators=(',', ':')) + '\n').encode()


def make_input(name, owners, messages, size):
    root = WORK / name / 'logs'
    root.mkdir(parents=True)
    for owner in range(owners):
        meta = json.dumps({'type': 'session_meta', 'payload': {'id': f'owner-{owner}', 'cwd': ''}}).encode() + b'\n'
        (root / f'rollout-{owner:04d}.jsonl').write_bytes(meta + b''.join(line(owner, i, size) for i in range(messages)))
    return root


def digest(db):
    with sqlite3.connect(db) as conn:
        assert conn.execute('PRAGMA integrity_check').fetchone()[0] == 'ok'
        data = []
        for table in ('inputs', 'sessions', 'messages', 'import_state'):
            columns = [r[1] for r in conn.execute(f'PRAGMA table_info({table})') if r[1] != 'imported_at']
            rows = sorted(conn.execute(f'SELECT {",".join(columns)} FROM {table}').fetchall(), key=repr)
            data.append((table, rows))
        count = conn.execute('SELECT count(*) FROM messages').fetchone()[0]
        states = conn.execute('SELECT count(*) FROM import_state').fetchone()[0]
    return hashlib.sha256(repr(data).encode()).hexdigest(), count, states


def run_process(args):
    started = time.monotonic()
    before = resource.getrusage(resource.RUSAGE_CHILDREN)
    proc = subprocess.Popen(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, start_new_session=True)
    try:
        stdout, stderr = proc.communicate(timeout=60)
    except subprocess.TimeoutExpired:
        os.killpg(proc.pid, signal.SIGKILL)
        proc.communicate()
        raise AssertionError(f'60s timeout: {args}')
    elapsed = time.monotonic() - started
    after = resource.getrusage(resource.RUSAGE_CHILDREN)
    assert proc.returncode == 0, (proc.returncode, stdout, stderr)
    assert '0 failed' in stdout and '0 unparsed lines' in stdout, stdout
    return {'wall_s': elapsed, 'user_s': after.ru_utime - before.ru_utime,
            'sys_s': after.ru_stime - before.ru_stime, 'stdout': stdout.strip(), 'stderr': stderr.strip(),
            'exit_code': proc.returncode}


def one(binary, root, label, name, phase, rep, owners, messages, size, probe=False):
    stem = f'{name}-{label}-{phase}-{rep}' + ('-probe' if probe else '')
    db = WORK / name / (stem + '.db')
    cfg = WORK / name / (stem + '.toml')
    cfg.write_text(f'db = "{db}"\n[[inputs]]\nsource = "codex"\nroot = "{root}"\n')
    cmd = [str(binary), 'import', '--config', str(cfg)]
    if phase not in ('initial',):
        run_process(cmd)
    edited = root / 'rollout-0000.jsonl'
    original = edited.read_bytes() if phase in ('append', 'same_size_edit') else None
    extra = root / 'rollout-extra.jsonl'
    try:
        if phase == 'append':
            edited.write_bytes(original + line(0, messages, 64))
        if phase == 'same_size_edit':
            changed = original.replace(b'0-0-', b'0-Z-', 1)
            assert len(changed) == len(original) and changed != original
            edited.write_bytes(changed)
        if phase == 'new_rollout':
            extra.write_bytes(json.dumps({'type': 'session_meta', 'payload': {'id': 'owner-0', 'cwd': ''}}).encode() + b'\n' + line(0, messages, 64))
        measured = run_process(cmd + (['--full', '--yes'] if phase == 'full' else []))
        sha, count, states = digest(db)
        expected = owners * messages + int(phase in ('append', 'new_rollout'))
        expected_states = owners + int(phase == 'new_rollout')
        assert (count, states) == (expected, expected_states), (phase, count, states, expected, expected_states)
        if probe:
            measured['peak_rss_bytes'] = int(re.search(r'PERF_RSS (\d+)', measured['stderr'])[1])
            measured['logical_read_bytes'] = int(re.search(r'PERF_BYTES (\d+)', measured['stderr'])[1])
            assert measured['peak_rss_bytes'] < 1024**3
            measured.pop('stderr')
        measured.update(version=label, case=name, phase=phase, repetition=rep, digest=sha, messages=count, import_states=states, probe=probe)
        return measured
    finally:
        if original is not None:
            edited.write_bytes(original)
        extra.unlink(missing_ok=True)
        for path in (db, cfg, Path(str(db) + '-wal'), Path(str(db) + '-shm')):
            path.unlink(missing_ok=True)


def check_budget(started):
    assert time.monotonic() - started < 900, '15 minute measurement budget exceeded'
    bytes_used = sum(p.stat().st_size for p in WORK.rglob('*') if p.is_file())
    assert bytes_used < 2 * 1024**3, f'2 GiB scratch budget exceeded: {bytes_used}'


def main():
    assert not WORK.exists(), f'fresh scratch path required: {WORK}'
    assert subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=REPO, text=True).strip() == CURRENT
    WORK.mkdir(parents=True)
    os.environ['GOCACHE'] = str(WORK / 'go-cache')
    started = time.monotonic()
    sources = {label: make_source(rev, label) for label, rev in (('baseline', BASELINE), ('current', CURRENT))}
    binaries = {label: build(source, label) for label, source in sources.items()}
    probes = {label: build(source, label, probe=True) for label, source in sources.items()}
    report = {'baseline': BASELINE, 'current': CURRENT, 'go': subprocess.check_output(['go', 'version'], text=True).strip(),
              'os': platform.platform(), 'environment': {k: os.environ.get(k) for k in ('GOGC', 'GOMEMLIMIT', 'GOMAXPROCS', 'GOFLAGS', 'GOCACHE')},
              'cases': [], 'runs': [], 'probe_runs': []}
    for name, owners, messages, size in CASES:
        root = make_input(name, owners, messages, size)
        report['cases'].append({'name': name, 'owners': owners, 'messages_per_owner': messages,
                                'input_bytes': sum(p.stat().st_size for p in root.iterdir()),
                                'max_owner_bytes': max(p.stat().st_size for p in root.iterdir())})
        for rep in range(3):
            for label in (('baseline', 'current') if rep % 2 == 0 else ('current', 'baseline')):
                for phase in PHASES:
                    result = one(binaries[label], root, label, name, phase, rep, owners, messages, size)
                    report['runs'].append(result)
                    print(name, rep, label, phase, round(result['wall_s'], 3), flush=True)
                    check_budget(started)
        for label in ('baseline', 'current'):
            for phase in PHASES:
                result = one(probes[label], root, label, name, phase, 0, owners, messages, size, probe=True)
                report['probe_runs'].append(result)
                check_budget(started)
        (OUT / 'results.json').write_text(json.dumps(report, indent=2) + '\n')
        check_budget(started)
    for name, _, _, _ in CASES:
        for phase in PHASES:
            assert len({r['digest'] for r in report['runs'] if r['case'] == name and r['phase'] == phase}) == 1, (name, phase)
            assert len({r['digest'] for r in report['probe_runs'] if r['case'] == name and r['phase'] == phase}) == 1, (name, phase)
    report['total_s'] = time.monotonic() - started
    (OUT / 'results.json').write_text(json.dumps(report, indent=2) + '\n')


def probe_only():
    assert WORK.exists()
    os.environ['GOCACHE'] = str(WORK / 'go-cache')
    report = json.loads((OUT / 'results.json').read_text())
    assert report['baseline'] == BASELINE and report['current'] == CURRENT
    started = time.monotonic()
    probes = {label: build(WORK / f'{label}-source', label, probe=True) for label in ('baseline', 'current')}
    report['probe_runs'] = []
    for name, owners, messages, size in CASES:
        root = WORK / name / 'logs'
        for label in ('baseline', 'current'):
            for phase in PHASES:
                report['probe_runs'].append(one(probes[label], root, label, name, phase, 0, owners, messages, size, probe=True))
                check_budget(started)
        (OUT / 'results.json').write_text(json.dumps(report, indent=2) + '\n')
    for name, _, _, _ in CASES:
        for phase in PHASES:
            assert len({r['digest'] for r in report['probe_runs'] if r['case'] == name and r['phase'] == phase}) == 1, (name, phase)


def repeat_edit():
    assert WORK.exists()
    os.environ['GOCACHE'] = str(WORK / 'go-cache')
    report = json.loads((OUT / 'results.json').read_text())
    started = time.monotonic()
    runs = []
    for rep in range(3, 6):
        for label in (('current', 'baseline') if rep % 2 else ('baseline', 'current')):
            result = one(WORK / label, WORK / 'many_body' / 'logs', label,
                         'many_body', 'same_size_edit', rep, 64, 8, 65536)
            runs.append(result)
            print(rep, label, result['wall_s'], result['user_s'] + result['sys_s'], flush=True)
            check_budget(started)
    assert len({r['digest'] for r in runs}) == 1
    assert runs[0]['digest'] == next(r['digest'] for r in report['runs'] if r['case'] == 'many_body' and r['phase'] == 'same_size_edit')
    report['followup_runs'] = runs
    (OUT / 'results.json').write_text(json.dumps(report, indent=2) + '\n')


if __name__ == '__main__':
    if sys.argv[1:] == ['--probe-only']:
        probe_only()
    elif sys.argv[1:] == ['--repeat-edit']:
        repeat_edit()
    else:
        main()
