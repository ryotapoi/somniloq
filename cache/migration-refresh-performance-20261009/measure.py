#!/usr/bin/env python3
"""Refresh all cumulative migration cases against the pre-optimization commit."""
import importlib.util
import io
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import sys
import tarfile
import time

sys.dont_write_bytecode = True
REPO = Path.cwd().resolve()
WORK = Path(os.environ.get('SOMNILOQ_PERF_WORK', str(REPO / '.tmp/performance-refresh-20261009'))).resolve()
BEFORE = '6cfd9d53a3bf7029e8f034aee5d26e8cc619d17e'
spec = importlib.util.spec_from_file_location('migration', REPO / 'cache/migration-performance-20261008/measure.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
m.WORK = WORK / 'inputs'
m.CASES += [('giant_group', 1, 1, 1024, 65536, 1, 0)]


def digest_tables(path):
    """Compare persisted rows except the intentionally varying import timestamp."""
    with sqlite3.connect(path) as db:
        result = {}
        for (table,) in db.execute("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name"):
            columns = [r[1] for r in db.execute(f'PRAGMA table_info("{table}")') if r[1] != 'imported_at']
            names = ','.join('"' + c + '"' for c in columns)
            rows = db.execute(f'SELECT {names} FROM "{table}"').fetchall()
            result[table] = sorted(rows, key=repr)
        return result


def main():
    WORK.mkdir(parents=True, exist_ok=True)
    assert not m.WORK.exists(), 'Use a new scratch directory; do not overwrite prior runs'
    m.WORK.mkdir()
    os.environ['GOCACHE'] = str(REPO / '.tmp/migration-cumulative-performance/go-cache')
    os.environ['GOFLAGS'] = '-buildvcs=false'
    current = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    report = dict(before_commit=BEFORE, after_commit=current,
                  go=subprocess.check_output(['go', 'version'], text=True).strip(),
                  os=subprocess.check_output(['sw_vers'], text=True).strip(),
                  started_at=time.strftime('%Y-%m-%dT%H:%M:%S%z'),
                  gc_environment={k: os.environ.get(k) for k in ['GOGC', 'GOMEMLIMIT']},
                  cases=[])
    for version, commit in [('before', BEFORE), ('after', current)]:
        source = WORK / (version + '-source')
        source.mkdir()
        archive = subprocess.check_output(['git', 'archive', commit, 'go.mod', 'go.sum', 'internal', 'cmd'])
        with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
            tar.extractall(source, filter='data')
        m.command(['go', 'build', '-o', str(WORK / version), './cmd/somniloq'], cwd=source)
    for case in m.CASES:
        base, details = m.setup(case)
        runs = []
        for repetition in range(3):
            expected = {}
            versions = ['before', 'after'] if repetition % 2 == 0 else ['after', 'before']
            for version in versions:
                for path in base.glob('new.db*'):
                    path.unlink()
                for retry in [False, True]:
                    result = m.run(WORK / version, base, details, retry, False)
                    rows = digest_tables(base / 'new.db')
                    if retry in expected:
                        assert rows == expected[retry], (case[0], repetition, retry, 'persisted rows differ')
                    else:
                        expected[retry] = rows
                    result.update(version=version, repetition=repetition, exit_code=0, preservation_verified=True)
                    runs.append(result)
                    print(case[0], repetition, version, 'retry' if retry else 'first', result['wall_s'], round(result['peak_rss_bytes']/1048576, 2), flush=True)
        report['cases'].append(dict(spec=details, runs=runs, paired_rows_equal=True))
        report['work_bytes'] = sum(p.stat().st_size for p in WORK.rglob('*') if p.is_file())
        (WORK / 'results.json').write_text(json.dumps(report, indent=2) + '\n')
        if report['work_bytes'] > 2*1024**3:
            raise RuntimeError('disk budget exceeded')
        if time.monotonic() - started > 900:
            raise RuntimeError('total measurement budget exceeded')
    report['finished_at'] = time.strftime('%Y-%m-%dT%H:%M:%S%z')
    (WORK / 'results.json').write_text(json.dumps(report, indent=2) + '\n')


if __name__ == '__main__':
    started = time.monotonic()
    main()
