import hashlib
import importlib.util
import json
import os
import sqlite3
import subprocess
import sys
import time
from pathlib import Path

sys.dont_write_bytecode = True
repo = Path.cwd().resolve()
work = repo / '.tmp/snapshot-hash'
spec = importlib.util.spec_from_file_location('baseline_measure', repo / 'cache/migration-performance-20261008/measure.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
m.WORK = work / 'inputs'
assert not m.WORK.exists()
m.WORK.mkdir()
m.CASES = [
    ('giant_group', 1, 1, 1024, 65536, 1, 0),
    ('body64MiB', 16, 1, 64, 65536, 1, 0),
    ('short_body', 16, 1, 64, 512, 1, 0),
]
os.environ['GOFLAGS'] = '-buildvcs=false'
os.environ['GOCACHE'] = str(repo / '.tmp/migration-cumulative-performance/go-cache')


def saved_digest(path):
    digest = hashlib.sha256()
    db = sqlite3.connect(path)
    for (table,) in db.execute("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name"):
        cols = [row[1] for row in db.execute(f'PRAGMA table_info({table})') if row[1] != 'imported_at']
        # imported_at is the only run-time value in these migration tables.
        rows = db.execute(f'SELECT {",".join(cols)} FROM {table} ORDER BY rowid')
        digest.update(table.encode())
        for row in rows:
            digest.update(json.dumps(row, ensure_ascii=False, separators=(',', ':')).encode())
            digest.update(b'\n')
    db.close()
    return digest.hexdigest()


report = {
    'baseline_commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip(),
    'go': subprocess.check_output(['go', 'version'], text=True).strip(),
    'os': subprocess.check_output(['sw_vers'], text=True).strip(),
    'cases': [],
    'started_at': time.strftime('%Y-%m-%dT%H:%M:%S%z'),
}
for case in m.CASES:
    base, details = m.setup(case)
    runs = []
    for repetition in range(3):
        order = ['baseline', 'candidate'] if repetition % 2 == 0 else ['candidate', 'baseline']
        saved = {}
        for version in order:
            for path in base.glob('new.db*'):
                path.unlink()
            for retry in [False, True]:
                result = m.run(work / version, base, details, retry, False)
                assert result['summary']['snapshot_sha256'] == hashlib.sha256((base / 'old.db').read_bytes()).hexdigest()
                fingerprint = saved_digest(base / 'new.db')
                saved[version, retry] = fingerprint
                result.update(version=version, repetition=repetition, exit_code=0, preservation_verified=True, saved_digest=fingerprint)
                runs.append(result)
                print(case[0], repetition, version, 'retry' if retry else 'first', result['wall_s'], round(result['peak_rss_bytes']/1048576, 2), flush=True)
        for retry in [False, True]:
            assert saved['baseline', retry] == saved['candidate', retry], (case[0], repetition, retry)
        (work / 'results.json').write_text(json.dumps(report, indent=2) + '\n')
    report['cases'].append(dict(spec=details, runs=runs))
    report['work_bytes'] = sum(p.stat().st_size for p in work.rglob('*') if p.is_file())
    (work / 'results.json').write_text(json.dumps(report, indent=2) + '\n')
    if report['work_bytes'] > 2 * 1024**3:
        raise RuntimeError('disk budget exceeded')
report['finished_at'] = time.strftime('%Y-%m-%dT%H:%M:%S%z')
(work / 'results.json').write_text(json.dumps(report, indent=2) + '\n')
