#!/usr/bin/env python3
"""Measure unchanged import allocations using the same synthetic shape."""
import json
from pathlib import Path
import shutil
import subprocess

REPO = Path(__file__).resolve().parents[2]
WORK = REPO / '.tmp/codex-unchanged-import-allocation-20261010'
BENCH = Path(__file__).with_name('alloc_bench.go.txt')
BASELINE = 'c4af9909e4499860c8289db76d1909042a860373'


def main():
    assert not WORK.exists(), f'remove scratch directory first: {WORK}'
    WORK.mkdir(parents=True)
    baseline = WORK / 'baseline'
    baseline.mkdir()
    archive = subprocess.check_output(['git', 'archive', BASELINE], cwd=REPO)
    subprocess.run(['tar', '-x', '-C', str(baseline)], input=archive, check=True)
    candidate = WORK / 'candidate'
    candidate.mkdir()
    for name in ('go.mod', 'go.sum'):
        shutil.copy(REPO / name, candidate / name)
    shutil.copytree(REPO / 'internal', candidate / 'internal')
    for name, source in (('baseline', baseline), ('candidate', candidate)):
        shutil.copy(BENCH, source / 'internal/core/alloc_bench_test.go')
    results = []
    for repetition in range(3):
        order = ('baseline', 'candidate') if repetition % 2 == 0 else ('candidate', 'baseline')
        for name in order:
            source = baseline if name == 'baseline' else candidate
            proc = subprocess.run(['go', 'test', './internal/core', '-run', '^$', '-bench', '^BenchmarkCodexUnchangedImportAlloc$',
                                   '-benchtime=5x', '-count=1', '-benchmem'], cwd=source, capture_output=True, text=True, timeout=90)
            assert proc.returncode == 0, (proc.stdout, proc.stderr)
            results.append({'version': name, 'repetition': repetition, 'output': proc.stdout})
            print(name, repetition, proc.stdout, flush=True)
    Path(__file__).with_name('allocation-results.json').write_text(json.dumps(results, indent=2) + '\n')


if __name__ == '__main__':
    main()
