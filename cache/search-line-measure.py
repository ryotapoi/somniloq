import base64
import hashlib
import json
import os
from pathlib import Path
import statistics
import subprocess
import time

ROOT = Path('.tmp/search-line-benchmark')
CONFIG = ROOT / 'config.toml'
CASES = [
    ('dense_long', 0), ('dense_long', 5), ('dense_medium', 5), ('dense_short', None),
    ('sparse_long', 5), ('no_match', 5), ('multiline', 5),
]

def wait_with_budget(pid, seconds):
    deadline = time.monotonic() + seconds
    while True:
        completed, status, usage = os.wait4(pid, os.WNOHANG)
        if completed:
            return os.waitstatus_to_exitcode(status), usage
        if time.monotonic() >= deadline:
            os.kill(pid, 9)
            os.wait4(pid, 0)
            raise TimeoutError(f'{pid} exceeded {seconds}s')
        time.sleep(0.002)

listing = json.loads(subprocess.check_output(
    [str(ROOT / 'somniloq-before'), 'search', '--config', str(CONFIG), '--format', 'json'],
    timeout=10,
))
refs = {json.loads(base64.urlsafe_b64decode(item['ref'].split(':')[-1] + '==='))[0]: item['ref'] for item in listing['items']}
records = []
for name, limit in CASES:
    for round_number in range(6):
        order = ('before', 'after') if round_number % 2 == 0 else ('after', 'before')
        for side in order:
            command = [str(ROOT / f'somniloq-{side}'), 'search', '--config', str(CONFIG), '--session', refs[name], '--format', 'json', '-F', 'a']
            if limit is not None:
                command[2:2] = ['--limit', str(limit)]
            destination = ROOT / f'{name}-{limit}-{side}.json'
            with destination.open('wb') as output:
                start = time.perf_counter()
                process = subprocess.Popen(command, stdout=output, stderr=subprocess.PIPE)
                code, usage = wait_with_budget(process.pid, 15)
                wall_ms = (time.perf_counter() - start) * 1000
                stderr = process.stderr.read()
            if code:
                raise RuntimeError((command, code, stderr))
            data = destination.read_bytes()
            if len(data) > 256 * 1024 * 1024:
                raise RuntimeError(('output too large', name, len(data)))
            page = json.loads(data)
            rec = {'case': name, 'limit': limit, 'round': round_number, 'side': side,
                   'wall_ms': round(wall_ms, 3), 'cpu_ms': round((usage.ru_utime + usage.ru_stime) * 1000, 3),
                   'rss_mib': round(usage.ru_maxrss / 1048576, 3),
                   'sha256': hashlib.sha256(data).hexdigest(), 'bytes': len(data),
                   'total': page['total'], 'count': page['count']}
            records.append(rec)
            print(json.dumps(rec), flush=True)
    group = [r for r in records if r['case'] == name and r['limit'] == limit]
    if len({r['sha256'] for r in group}) != 1:
        raise RuntimeError(('outputs differ', name, limit))

(ROOT / 'results.jsonl').write_text('\n'.join(json.dumps(r) for r in records) + '\n', encoding='utf-8')
for name, limit in CASES:
    group = [r for r in records if r['case'] == name and r['limit'] == limit]
    for side in ('before', 'after'):
        vals = [r for r in group if r['side'] == side]
        print(name, limit, side, 'wall', statistics.median(r['wall_ms'] for r in vals),
              'range', (min(r['wall_ms'] for r in vals), max(r['wall_ms'] for r in vals)),
              'cpu', statistics.median(r['cpu_ms'] for r in vals),
              'rss', statistics.median(r['rss_mib'] for r in vals))
