#!/usr/bin/env python3
"""Paired Codex import measurements on isolated synthetic inputs and DBs."""
import hashlib
import json
import os
from pathlib import Path
import platform
import sqlite3
import subprocess
import time

REPO = Path(__file__).resolve().parents[2]
BASELINE = 'c4af9909e4499860c8289db76d1909042a860373'
WORK = REPO / '.tmp/codex-unchanged-import-performance-20261010'
OWNERS = 32
MESSAGES = 80
PADDING = 'x' * 512


def run_command(args, timeout=60):
    result = subprocess.run(args, capture_output=True, text=True, timeout=timeout)
    assert result.returncode == 0, (args, result.stdout, result.stderr)
    return result


def line(owner, number):
    return json.dumps({'type': 'response_item', 'timestamp': '2026-01-01T00:00:00Z',
                       'payload': {'type': 'message', 'role': 'user', 'id': f'{owner}-{number}',
                                   'content': [{'type': 'input_text', 'text': f'{owner}-{number}-{PADDING}'}]}},
                      separators=(',', ':')) + '\n'


def make_input():
    root = WORK / 'logs'
    root.mkdir(parents=True)
    for owner in range(OWNERS):
        text = json.dumps({'type': 'session_meta', 'payload': {'id': f'owner-{owner}', 'cwd': ''}}) + '\n'
        text += ''.join(line(owner, i) for i in range(MESSAGES))
        (root / f'rollout-{owner:03d}.jsonl').write_text(text)
    return root


def db_digest(path):
    with sqlite3.connect(path) as db:
        assert db.execute('PRAGMA integrity_check').fetchone()[0] == 'ok'
        data = []
        for table in ('sessions', 'messages', 'import_state'):
            columns = [r[1] for r in db.execute(f'PRAGMA table_info({table})') if r[1] != 'imported_at']
            data.append((table, sorted(db.execute(f'SELECT {",".join(columns)} FROM {table}').fetchall(), key=repr)))
        count = db.execute('SELECT count(*) FROM messages').fetchone()[0]
    return hashlib.sha256(repr(data).encode()).hexdigest(), count


def measure(binary, root, version, phase, repetition):
    base = WORK / f'{version}-{phase}-{repetition}'
    cfg, db = base.with_suffix('.toml'), base.with_suffix('.db')
    cfg.write_text(f'db = "{db}"\n[[inputs]]\nsource = "codex"\nroot = "{root}"\n')
    cmd = [str(binary), 'import', '--config', str(cfg)]
    if phase != 'initial':
        run_command(cmd)
    edited = root / 'rollout-000.jsonl'
    original = edited.read_bytes()
    extra = root / 'rollout-extra.jsonl'
    try:
        if phase == 'append':
            edited.write_bytes(original + line(0, MESSAGES).encode())
        elif phase == 'same_size_edit':
            changed = original.replace(b'0-0-', b'0-Z-', 1)
            assert len(changed) == len(original)
            edited.write_bytes(changed)
        elif phase == 'new_rollout':
            extra.write_text(json.dumps({'type': 'session_meta', 'payload': {'id': 'owner-0', 'cwd': ''}}) + '\n' + line(0, MESSAGES))
        if phase == 'full':
            cmd += ['--full', '--yes']
        began = time.perf_counter()
        proc = run_command(cmd)
        elapsed = time.perf_counter() - began
        digest, count = db_digest(db)
        expected = OWNERS * MESSAGES + int(phase in ('append', 'new_rollout'))
        assert count == expected, (phase, count, expected)
        assert '0 failed' in proc.stdout and '0 unparsed lines' in proc.stdout, proc.stdout
        return {'version': version, 'phase': phase, 'repetition': repetition,
                'wall_s': elapsed, 'messages': count,
                'db_sha256': digest, 'stdout': proc.stdout.strip()}
    finally:
        edited.write_bytes(original)
        extra.unlink(missing_ok=True)


def main():
    assert not WORK.exists(), f'remove scratch directory first: {WORK}'
    WORK.mkdir(parents=True)
    snapshot = WORK / 'baseline-source'
    snapshot.mkdir()
    archive = subprocess.check_output(['git', 'archive', BASELINE], cwd=REPO)
    subprocess.run(['tar', '-x', '-C', str(snapshot)], input=archive, check=True)
    baseline = WORK / 'baseline'
    candidate = WORK / 'candidate'
    subprocess.run(['go', 'build', '-o', str(baseline), './cmd/somniloq'], cwd=snapshot, check=True)
    subprocess.run(['go', 'build', '-o', str(candidate), './cmd/somniloq'], cwd=REPO, check=True)
    root = make_input()
    input_bytes = sum(p.stat().st_size for p in root.iterdir())
    report = {'baseline_commit': subprocess.check_output(['git', 'rev-parse', BASELINE], cwd=REPO, text=True).strip(),
              'candidate': 'working tree', 'go': subprocess.check_output(['go', 'version'], text=True).strip(),
              'os': platform.platform(), 'input_bytes': input_bytes, 'owners': OWNERS,
              'messages_per_owner': MESSAGES, 'max_owner_bytes': max(p.stat().st_size for p in root.iterdir()),
              'environment': {k: os.environ.get(k) for k in ('GOGC', 'GOMEMLIMIT', 'GOMAXPROCS', 'GOFLAGS')}, 'runs': []}
    start = time.monotonic()
    for phase in ('unchanged', 'append', 'same_size_edit', 'new_rollout', 'initial', 'full'):
        for repetition in range(3):
            order = ('baseline', 'candidate') if repetition % 2 == 0 else ('candidate', 'baseline')
            paired = []
            for version in order:
                result = measure(baseline if version == 'baseline' else candidate, root, version, phase, repetition)
                report['runs'].append(result)
                paired.append(result)
                print(phase, version, repetition, round(result['wall_s'], 3), flush=True)
            assert paired[0]['db_sha256'] == paired[1]['db_sha256'], phase
            assert time.monotonic() - start < 180
            assert sum(p.stat().st_size for p in WORK.rglob('*') if p.is_file()) < 1024**3
    (Path(__file__).with_name('results.json')).write_text(json.dumps(report, indent=2) + '\n')


if __name__ == '__main__':
    main()
