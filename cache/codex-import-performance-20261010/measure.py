#!/usr/bin/env python3
"""Paired ordinary Codex import measurements using synthetic logs and databases."""
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import sqlite3
import subprocess
import time

REPO = Path(__file__).resolve().parents[2]
WORK = REPO / '.tmp/codex-import-performance-20261010'
BASELINE = 'cd4ac1316849fb44d640bc8abe6a9dbb6c7202c9'
CASES = [('owners16', 16, 2, 262144, False), ('owners64', 64, 2, 262144, False), ('owners64_body', 64, 8, 65536, True), ('one_large', 1, 128, 262144, False)]


def make_case(name, owners, padding_lines, padding_size, body):
    root = WORK / name / 'logs'
    root.mkdir(parents=True)
    for owner in range(owners):
        records = [{'type': 'session_meta', 'payload': {'id': f'owner-{owner}', 'cwd': ''}}]
        if body:
            records += [{'type': 'response_item', 'timestamp': '2026-01-01T00:00:00Z', 'payload': {'type': 'message', 'role': 'user', 'id': f'm{owner}-{i}', 'content': [{'type': 'input_text', 'text': 'x' * padding_size}]}} for i in range(padding_lines)]
        else:
            records += [{'type': 'event_msg', 'payload': {'padding': 'x' * padding_size}} for _ in range(padding_lines)]
        records += [{'type': 'response_item', 'timestamp': '2026-01-01T00:00:00Z', 'payload': {'type': 'message', 'role': 'user', 'content': [{'type': 'input_text', 'text': f'body-{owner}'}]}}]
        (root / f'rollout-{owner:04d}.jsonl').write_text(''.join(json.dumps(r, separators=(',', ':')) + '\n' for r in records))
    return root, sum(p.stat().st_size for p in root.iterdir())


def digest(conn):
    contents = []
    for table in ['inputs', 'sessions', 'messages', 'import_state']:
        columns = [row[1] for row in conn.execute(f'PRAGMA table_info({table})') if row[1] != 'imported_at']
        rows = sorted(conn.execute(f'SELECT {",".join(columns)} FROM {table}').fetchall(), key=repr)
        contents.append((table, rows))
    assert conn.execute('PRAGMA integrity_check').fetchone()[0] == 'ok'
    return hashlib.sha256(repr(contents).encode()).hexdigest()


def run(binary, name, root, version, phase, repetition, total_bytes, messages):
    db = WORK / name / f'{version}-{phase}-{repetition}.db'
    cfg = WORK / name / f'{version}-{phase}-{repetition}.toml'
    cfg.write_text(f'db = "{db}"\n[[inputs]]\nsource = "codex"\nroot = "{root}"\n')
    cmd = [str(binary), 'import', '--config', str(cfg)]
    if phase == 'full':
        seeded = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
        assert seeded.returncode == 0, (seeded.stdout, seeded.stderr)
        cmd.extend(['--full', '--yes'])
    started = time.monotonic()
    proc = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
    stdout, stderr = proc.stdout, proc.stderr
    elapsed = time.monotonic() - started
    assert proc.returncode == 0, (stdout, stderr)
    assert f'Imported {len(list(root.iterdir()))} files' in stdout and '0 failed' in stdout, stdout
    with sqlite3.connect(db) as conn:
        assert conn.execute('SELECT count(*) FROM messages').fetchone()[0] == messages
        assert conn.execute('SELECT count(*) FROM import_state').fetchone()[0] == len(list(root.iterdir()))
        rows_sha256 = digest(conn)
    rss = int(re.search(r'PERF_RSS (\d+)', stderr)[1])
    read_bytes = int(re.search(r'PERF_BYTES (\d+)', stderr)[1])
    assert rss < 1024**3, rss
    return {'version': version, 'phase': phase, 'repetition': repetition, 'wall_s': elapsed, 'peak_rss_bytes': rss, 'input_bytes': total_bytes, 'read_bytes': read_bytes, 'rows_sha256': rows_sha256, 'stdout': stdout.strip()}


def main():
    assert not WORK.exists(), 'Use a fresh scratch directory'
    WORK.mkdir(parents=True)
    def build(source, version):
        copied = WORK / f'{version}-source'
        copied.mkdir()
        for name in ['go.mod', 'go.sum']:
            shutil.copy(source / name, copied / name)
        for name in ['cmd', 'internal']:
            shutil.copytree(source / name, copied / name)
        source = copied
        probe = source / 'internal/perfprobe'
        probe.mkdir()
        (probe / 'probe.go').write_text('''package perfprobe
import ("io"; "os")
var Bytes int64
func ReadFile(path string) ([]byte,error) { b,e:=os.ReadFile(path); Bytes+=int64(len(b)); return b,e }
type Reader struct { io.Reader }
func (r Reader) Read(p []byte) (int,error) { n,e:=r.Reader.Read(p); Bytes+=int64(n); return n,e }
''')
        group = source / 'internal/ingest/codex/group.go'
        group_text = group.read_text().replace('import (', 'import (\n"github.com/ryotapoi/somniloq/internal/perfprobe"').replace('os.ReadFile(path)', 'perfprobe.ReadFile(path)')
        if version == 'candidate':
            assert 'r := bufio.NewReader(f)' in group_text
            group_text = group_text.replace('r := bufio.NewReader(f)', 'r := bufio.NewReader(perfprobe.Reader{Reader:f})')
        group.write_text(group_text)
        original = source / 'cmd/somniloq/main.go'
        text = original.read_text().replace('import (', 'import (\n"syscall"\n"github.com/ryotapoi/somniloq/internal/perfprobe"').replace('os.Exit(code)', 'var usage syscall.Rusage\n syscall.Getrusage(syscall.RUSAGE_SELF, &usage)\n fmt.Fprintf(os.Stderr, "PERF_RSS %d\\nPERF_BYTES %d\\n", usage.Maxrss, perfprobe.Bytes)\n os.Exit(code)')
        original.write_text(text)
        binary = WORK / version
        subprocess.run(['go', 'build', '-o', str(binary), './cmd/somniloq'], cwd=source, check=True)
        return binary
    snapshot = WORK / 'snapshot'
    snapshot.mkdir()
    archive = subprocess.check_output(['git', 'archive', BASELINE], cwd=REPO)
    subprocess.run(['tar', '-x', '-C', str(snapshot)], input=archive, check=True)
    baseline = build(snapshot, 'baseline')
    candidate = build(REPO, 'candidate')
    results = {'baseline': BASELINE,
               'go': subprocess.check_output(['go', 'version'], text=True).strip(), 'os': platform.platform(),
               'environment': {key: os.environ.get(key) for key in ['GOGC', 'GOMEMLIMIT', 'GOMAXPROCS', 'GOFLAGS', 'GOCACHE']}, 'cases': []}
    began = time.monotonic()
    for name, owners, padding_lines, padding_size, body in CASES:
        root, total_bytes = make_case(name, owners, padding_lines, padding_size, body)
        runs = []
        for repetition in range(3):
            for version in (['baseline', 'candidate'] if repetition % 2 == 0 else ['candidate', 'baseline']):
                for phase in ['initial', 'full']:
                    run_result = run(baseline if version == 'baseline' else candidate, name, root, version, phase, repetition, total_bytes, owners * (padding_lines + 1 if body else 1))
                    runs.append(run_result)
                    print(name, version, phase, repetition, round(run_result['wall_s'], 3), run_result['peak_rss_bytes'], flush=True)
        assert len({r['rows_sha256'] for r in runs}) == 1
        results['cases'].append({'name': name, 'owners': owners, 'max_owner_bytes': max(p.stat().st_size for p in root.iterdir()), 'input_bytes': total_bytes, 'runs': runs})
        (WORK / 'results.json').write_text(json.dumps(results, indent=2) + '\n')
        assert time.monotonic() - began < 900
        assert sum(p.stat().st_size for p in WORK.rglob('*') if p.is_file()) < 2 * 1024**3
    shutil.copy(WORK / 'results.json', REPO / 'cache/codex-import-performance-20261010/results.json')


if __name__ == '__main__':
    main()
