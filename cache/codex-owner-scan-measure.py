"""Reproduce paired Codex owner scans using isolated synthetic logs and a 30-second timeout."""

import pathlib, json, sqlite3, subprocess, time, shutil, hashlib, os
B = pathlib.Path('.tmp/codex-owner-scan').resolve()
B.mkdir(parents=True, exist_ok=True)

def record(i, j):
    return json.dumps({'type': 'response_item', 'timestamp': '2026-01-01T00:00:00Z', 'payload': {'type': 'message', 'id': f'm{i}-{j}', 'role': 'user', 'content': [{'type': 'input_text', 'text': 'x' * 256}]}}, separators=(',', ':')) + '\n'

def digest():
    c = sqlite3.connect(B / 'run.db')
    result = []
    for table in ['sessions', 'messages', 'import_state', 'legacy_sessions', 'legacy_messages']:
        cols = [r[1] for r in c.execute(f'pragma table_info({table})') if r[1] != 'imported_at']
        result.append(sorted(c.execute(f"SELECT {','.join(cols)} FROM {table}").fetchall(), key=repr))
    c.close()
    return hashlib.sha256(repr(result).encode()).hexdigest()

def run(v, mode):
    cmd = [str(B / v), mode, '--config', str(B / 'config.toml')]
    if mode == 'migrate':
        cmd += ['--from', str(B / 'legacy.db')]
    start = time.perf_counter()
    r = subprocess.run(cmd, capture_output=True, timeout=30)
    wall = time.perf_counter() - start
    if r.returncode or r.stderr:
        raise Exception(r.returncode, r.stdout, r.stderr)
    return dict(wall=wall, stdout=r.stdout.decode(), digest=digest())

def reset(seed=None):
    for p in B.glob('run.db*'):
        p.unlink()
    if seed:
        shutil.copyfile(B / seed, B / 'run.db')
for n, m in json.loads(os.getenv('SIZES', '[[4,8],[64,64],[256,64],[256,256]]')):
    root = B / 'logs'
    shutil.rmtree(root, ignore_errors=True)
    root.mkdir()
    for i in range(n):
        (root / f'{i:04}.jsonl').write_text(json.dumps({'type': 'session_meta', 'payload': {'id': f'owner-{i}', 'cwd': '/fixture'}}) + '\n' + ''.join((record(i, j) for j in range(m))))
    (B / 'config.toml').write_text(f'''db = "{B / 'run.db'}"\n[[inputs]]\nsource = "codex"\nroot = "{root}"\n''')
    (B / 'legacy.db').unlink(missing_ok=True)
    c = sqlite3.connect(B / 'legacy.db')
    sql = pathlib.Path('internal/ingest/testdata/v0.14.0-migration/legacy.sql').read_text().split('INSERT INTO')[0]
    c.executescript(sql)
    c.commit()
    c.close()
    for mode in [] if os.getenv('MISSING_ONLY') else json.loads(os.getenv('MODES', '["import","migrate"]')):
        runs = {v: [] for v in ['baseline', 'candidate']}
        for k in range(4):
            for v in ['baseline', 'candidate'] if k % 2 == 0 else ['candidate', 'baseline']:
                reset()
                r = run(v, mode)
                runs[v].append(r)
                print(json.dumps(dict(n=n, m=m, mode=mode, v=v, k=k, **r)), flush=True)
        assert len({(r['digest'], r['stdout']) for rs in runs.values() for r in rs}) == 1
    if os.getenv('NO_MISSING'):
        continue
    for i in range(n):
        (root / f'extra-{i:04}.jsonl').write_text(json.dumps({'type': 'session_meta', 'payload': {'id': f'owner-{i}', 'cwd': '/fixture'}}) + '\n' + record(i, m + 1))
    reset()
    run('baseline', 'import')
    shutil.copyfile(B / 'run.db', B / 'seed.db')
    for i in range(n):
        (root / f'extra-{i:04}.jsonl').unlink()
        with (root / f'{i:04}.jsonl').open('a') as f:
            f.write(record(i, m + 2))
    missing_runs = []
    for k in range(4):
        for v in ['baseline', 'candidate'] if k % 2 == 0 else ['candidate', 'baseline']:
            reset('seed.db')
            r = run(v, 'import')
            missing_runs.append(r)
            print(json.dumps(dict(n=n, m=m, mode='missing', v=v, k=k, **r)), flush=True)
    assert len({(r['digest'], r['stdout']) for r in missing_runs}) == 1
    reset()
