import importlib.util
import json
import os
import subprocess
import sys
import time
from pathlib import Path

sys.dont_write_bytecode = True
repo = Path.cwd().resolve()
work = repo / '.tmp/migration-bytes'
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
report = {
    'baseline_commit': '1c3698b1acd8ca3da6fc28b1ffa972fbaf33f122',
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
        for version in order:
            for path in base.glob('new.db*'):
                path.unlink()
            for retry in [False, True]:
                result = m.run(work / version, base, details, retry, False)
                result.update(version=version, repetition=repetition, exit_code=0, preservation_verified=True)
                runs.append(result)
                print(case[0], repetition, version, 'retry' if retry else 'first', result['wall_s'], round(result['peak_rss_bytes']/1048576, 2), flush=True)
                (work / 'results.json').write_text(json.dumps(report, indent=2) + '\n')
    report['cases'].append(dict(spec=details, runs=runs))
    report['work_bytes'] = sum(p.stat().st_size for p in work.rglob('*') if p.is_file())
    (work / 'results.json').write_text(json.dumps(report, indent=2) + '\n')
    if report['work_bytes'] > 2 * 1024**3:
        raise RuntimeError('disk budget exceeded')
report['finished_at'] = time.strftime('%Y-%m-%dT%H:%M:%S%z')
(work / 'results.json').write_text(json.dumps(report, indent=2) + '\n')
