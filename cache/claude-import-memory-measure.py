import json, os, pathlib, shutil, subprocess, sys, time
# Generated measurement helper; run from the repository root.
BASE = pathlib.Path.cwd() / '.tmp/claude-performance'
MAX_RSS = 512 * 1024 * 1024
MAX_SECONDS = 30

def run(binary, config):
    if sum(p.stat().st_size for p in BASE.rglob('*') if p.is_file()) > 1024 ** 3:
        raise RuntimeError('disk budget exceeded')
    start = time.perf_counter()
    outpath, errpath = BASE / 'stdout', BASE / 'stderr'
    with outpath.open('w') as out, errpath.open('w') as err:
        p = subprocess.Popen([str(BASE / binary), 'import', '--config', str(config)], stdout=out, stderr=err)
    next_monitor = start
    try:
        while True:
            pid, status, usage = os.wait4(p.pid, os.WNOHANG)
            if pid:
                p.returncode = os.waitstatus_to_exitcode(status)
                break
            if time.perf_counter() - start > MAX_SECONDS:
                p.kill(); p.wait(); raise RuntimeError('time budget exceeded')
            try:
                if time.perf_counter() < next_monitor:
                    time.sleep(0.001)
                    continue
                next_monitor = time.perf_counter() + 0.1
                rss = int(subprocess.check_output(['/bin/ps', '-o', 'rss=', '-p', str(p.pid)], stderr=subprocess.DEVNULL)) * 1024
                if rss > MAX_RSS:
                    p.kill(); p.wait(); raise RuntimeError('memory budget exceeded')
            except (ValueError, subprocess.CalledProcessError):
                pass
            time.sleep(0.001)
    finally:
        if p.returncode is None:
            p.kill()
            p.wait()
    elapsed = time.perf_counter() - start
    if usage.ru_maxrss > MAX_RSS:
        raise RuntimeError('memory budget exceeded between samples')
    if p.returncode or errpath.read_text():
        raise RuntimeError((p.returncode, outpath.read_text(), errpath.read_text()))
    if '0 scanned' in outpath.read_text():
        raise RuntimeError('fixture was not scanned')
    return {'wall': round(elapsed, 4), 'cpu': round(usage.ru_utime + usage.ru_stime, 4), 'rss': usage.ru_maxrss, 'stdout': outpath.read_text().strip()}

def fixture(shape, count=32, records=128):
    root = BASE / shape / 'logs'
    (root / 'project').mkdir(parents=True, exist_ok=True)
    for i in range(count):
        lines = []
        for j in range(records):
            content = 'x' * 8192 if shape == 'text' else [{'type': 'tool_result', 'tool_use_id': f'tool-{j}', 'content': 'x' * 8192}]
            lines.append(json.dumps({'type':'user', 'uuid':f'm-{j}', 'sessionId':f'session-{i}', 'timestamp':'2026-01-01T00:00:00Z', 'message':{'role':'user', 'content':content}}, separators=(',', ':')) + '\n')
        (root / 'project' / f'session-{i}.jsonl').write_text(''.join(lines))
    return root

if sys.argv[1] == 'prepare':
    count = int(sys.argv[2])
    for shape in ['text', 'tools']:
        root = fixture(shape, count)
        config = BASE / shape / 'config.toml'
        config.write_text(f'db = "{BASE / shape / "run.db"}"\n[[inputs]]\nsource = "claude-code"\nroot = "{root}"\n')
        for suffix in ['', '-wal', '-shm']:
            (BASE / shape / ('run.db' + suffix)).unlink(missing_ok=True)
        result = run('baseline', config)
        shutil.copyfile(BASE / shape / 'run.db', BASE / shape / 'seed.db')
        print(shape, sum(p.stat().st_size for p in root.rglob('*.jsonl')), result, flush=True)
elif sys.argv[1] == 'measure':
    binaries = sys.argv[2:]
    for shape in ['text', 'tools']:
        root = BASE / shape / 'logs'
        target = root / 'project' / 'session-0.jsonl'
        original = target.read_bytes()
        config = BASE / shape / 'config.toml'
        for mode in ['unchanged', 'append']:
            if mode == 'append':
                target.write_bytes(original + (json.dumps({'type':'assistant', 'uuid':'new', 'sessionId':'session-0', 'timestamp':'2026-01-02T00:00:00Z', 'message':{'role':'assistant','content':'small appended response'}}) + '\n').encode())
            for trial in range(-1, 5):
                order = binaries if trial % 2 == 0 else list(reversed(binaries))
                for binary in order:
                    for suffix in ['', '-wal', '-shm']:
                        (BASE / shape / ('run.db' + suffix)).unlink(missing_ok=True)
                    shutil.copyfile(BASE / shape / 'seed.db', BASE / shape / 'run.db')
                    result = run(binary, config)
                    expected = f'Imported {0 if mode == "unchanged" else 1} files (32 scanned, {32 if mode == "unchanged" else 31} skipped, 0 failed, 0 unparsed lines)'
                    if result['stdout'] != expected:
                        raise RuntimeError(result)
                    if trial >= 0:
                        print(json.dumps({'shape':shape,'mode':mode,'trial':trial,'binary':binary, **result}), flush=True)
            target.write_bytes(original)
