#!/usr/bin/env python3
"""Paired migration comparison using the existing bounded synthetic harness."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

sys.dont_write_bytecode = True

REPO = Path.cwd().resolve()
WORK = REPO / '.tmp/git-resolution-performance'
spec = importlib.util.spec_from_file_location('baseline', REPO / 'cache/migration-performance-20261008/measure.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
m.WORK = WORK / 'inputs'
m.OUT = WORK / 'results.json'
m.CASES = [
    ('base',16,1,64,512,1,0),
    ('files4',16,4,64,512,1,0),
    ('groups16',256,1,4,512,1,0),
    ('cwd16',16,1,64,512,16,0),
    ('cwd64',64,1,16,512,64,0),
    ('cwd256',256,1,4,512,256,0),
    ('total16',256,1,64,512,1,0),
    ('no_git_1024',1024,1,1,512,0,0),
]


def copy_source(target):
    target.mkdir()
    for name in ['go.mod','go.sum','internal','cmd']:
        src = REPO/name
        shutil.copytree(src,target/name) if src.is_dir() else shutil.copy(src,target/name)


def repeat_control_probe(report):
    case = next(c for c in report['cases'] if c['spec']['name'] == 'no_git_1024')
    base = m.WORK/'no_git_1024'
    runs = []
    for version in ['before','after']:
        for path in base.glob('new.db*'): path.unlink()
        for retry in [False,True]:
            result = m.run(WORK/(version+'-probe'),base,case['spec'],retry,True)
            result.update(version=version,repetition=4)
            runs.append(result)
    case['extra_probe_runs'] = runs


def main():
    # Before editing, save the baseline source at this exact path. For reruns,
    # extract the recorded baseline commit's go.mod/go.sum/internal/cmd here.
    before = WORK/'before-source'
    assert before.exists(), 'Baseline source must be preserved before edits'
    assert not m.WORK.exists(), 'Use a fresh owned inputs directory'
    m.WORK.mkdir(parents=True)
    os.environ['GOFLAGS'] = '-buildvcs=false'
    os.environ['GOCACHE'] = str(REPO/'.tmp/migration-performance/go-cache')
    copy_source(WORK/'after-source')
    for version in ['before','after']:
        source = WORK/(version+'-source')
        m.command(['go','build','-o',str(WORK/version),'./cmd/somniloq'],cwd=source)
        probe = WORK/(version+'-probe-source')
        shutil.copytree(source,probe)
        m.instrument(probe)
        m.command(['go','build','-o',str(WORK/(version+'-probe')),'./cmd/somniloq'],cwd=probe)
    report = {'commit':subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(), 'cases':[]}
    for case in m.CASES:
        base, details = m.setup(case)
        runs = []
        for repetition in range(4):
            measured = repetition == 3
            # Alternate ordering to avoid assigning all warmup/load bias to one version.
            versions = ['before','after'] if repetition % 2 == 0 else ['after','before']
            for version in versions:
                for path in base.glob('new.db*'): path.unlink()
                for retry in [False,True]:
                    result = m.run(WORK/(version+('-probe' if measured else '')),base,details,retry,measured)
                    result.update(version=version,repetition=repetition)
                    runs.append(result)
        report['cases'].append({'spec':details,'runs':runs})
        m.OUT.write_text(json.dumps(report,indent=2)+'\n')
        print(case[0], 'complete', flush=True)
        if sum(p.stat().st_size for p in WORK.rglob('*') if p.is_file()) > 2*1024**3: raise RuntimeError('disk budget exceeded')
    repeat_control_probe(report)
    m.OUT.write_text(json.dumps(report,indent=2)+'\n')
    print(m.OUT, flush=True)

if __name__ == '__main__': main()
