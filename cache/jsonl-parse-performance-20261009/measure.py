#!/usr/bin/env python3
"""Paired, bounded CLI comparison for migration JSONL parsing."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

sys.dont_write_bytecode = True
REPO = Path.cwd().resolve()
WORK = REPO / '.tmp/jsonl-performance'
spec = importlib.util.spec_from_file_location('baseline', REPO / 'cache/migration-performance-20261008/measure.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
m.WORK = WORK / 'inputs'
m.OUT = WORK / 'results.json'
m.CASES = [
    ('base', 16, 1, 64, 512, 1, 0),
    ('total16', 256, 1, 64, 512, 1, 0),
    ('ignored_large', 16, 1, 64, 512, 1, 16384),
    ('body64MiB', 16, 1, 64, 65536, 1, 0),
]


def copy_source(target):
    target.mkdir()
    for name in ['go.mod', 'go.sum', 'internal', 'cmd']:
        src = REPO / name
        shutil.copytree(src, target / name) if src.is_dir() else shutil.copy2(src, target / name)


def instrument(source):
    path = source / 'internal/core/migrate.go'
    text = path.read_text()
    assert '"runtime"' not in text
    text = text.replace('import (', 'import (\n"runtime"', 1)
    marker = 'func Migrate(from, destination string, inputs []Input) (*MigrationResult, error) {'
    assert text.count(marker) == 1
    text = text.replace(marker, marker + '''
defer func() { var stats runtime.MemStats; runtime.ReadMemStats(&stats); fmt.Fprintf(os.Stderr, "PERFPROBE {\\"total_alloc_bytes\\":%d}\\n", stats.TotalAlloc) }()
''', 1)
    path.write_text(text)
    m.command(['gofmt', '-w', str(path)])


def main():
    assert (WORK / 'before-source').exists(), 'Preserve baseline source before editing'
    assert not m.WORK.exists(), 'Remove only the owned inputs directory before rerunning'
    m.WORK.mkdir(parents=True)
    os.environ['GOCACHE'] = str(WORK / 'go-cache')
    os.environ['GOFLAGS'] = '-buildvcs=false'
    copy_source(WORK / 'after-source')
    for version in ['before', 'after']:
        source = WORK / (version + '-source')
        m.command(['go', 'build', '-o', str(WORK / version), './cmd/somniloq'], cwd=source)
        probe = WORK / (version + '-probe-source')
        shutil.copytree(source, probe)
        instrument(probe)
        m.command(['go', 'build', '-o', str(WORK / (version + '-probe')), './cmd/somniloq'], cwd=probe)
    report = dict(commit=subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip(),
                  go=subprocess.check_output(['go', 'version'], text=True).strip(),
                  os=subprocess.check_output(['sw_vers'], text=True).strip(), cases=[])
    for case in m.CASES:
        base, details = m.setup(case)
        runs = []
        for repetition in range(4):
            measured = repetition == 3
            versions = ['before', 'after'] if repetition % 2 == 0 else ['after', 'before']
            for version in versions:
                for path in base.glob('new.db*'):
                    path.unlink()
                for retry in [False, True]:
                    result = m.run(WORK / (version + ('-probe' if measured else '')), base, details, retry, measured)
                    result.update(version=version, repetition=repetition)
                    runs.append(result)
        report['cases'].append(dict(spec=details, runs=runs))
        m.OUT.write_text(json.dumps(report, indent=2) + '\n')
        print(case[0], 'complete', flush=True)
        if sum(p.stat().st_size for p in WORK.rglob('*') if p.is_file()) > 2 * 1024 ** 3:
            raise RuntimeError('disk budget exceeded')
    print(m.OUT, flush=True)


if __name__ == '__main__':
    main()
