#!/usr/bin/env python3
"""Bounded current-version measurement of migration legacy UUID lookup."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

sys.dont_write_bytecode = True
REPO = Path.cwd().resolve()
WORK = REPO / '.tmp/uuid-performance'
spec = importlib.util.spec_from_file_location('baseline', REPO / 'cache/migration-performance-20261008/measure.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
m.WORK = WORK / 'inputs'
m.OUT = WORK / 'results.json'
m.CASES = [
    ('base', 16, 1, 64, 512, 1, 0),
    ('total16', 256, 1, 64, 512, 1, 0),
    ('body64MiB', 16, 1, 64, 65536, 1, 0),
]


def copy_source(target):
    target.mkdir()
    for name in ['go.mod', 'go.sum', 'internal', 'cmd']:
        src = REPO / name
        shutil.copytree(src, target / name) if src.is_dir() else shutil.copy2(src, target / name)


def instrument(source):
    probe = source / 'internal/perfprobe'
    probe.mkdir()
    (probe / 'probe.go').write_text('package perfprobe\nimport ("encoding/json"; "fmt"; "os"; "runtime"; "time")\nvar Counts = map[string]int64{}\nvar Seconds = map[string]float64{}\nfunc Start(name string) func() { t := time.Now(); return func(){Seconds[name] += time.Since(t).Seconds()} }\nfunc Emit(){ var m runtime.MemStats; runtime.ReadMemStats(&m); b,_ := json.Marshal(map[string]any{"counts":Counts,"seconds":Seconds,"total_alloc_bytes":m.TotalAlloc}); fmt.Fprintln(os.Stderr,"PERFPROBE "+string(b)) }\n')
    path = source / 'internal/core/migrate.go'
    def replace(old, new):
        m.replace(path, old, new)
    replace('import (', 'import (\n"github.com/ryotapoi/somniloq/internal/perfprobe"')
    replace('\tdb, digest, copied, err := prepareMigration', '\tdefer perfprobe.Emit()\n\tdb, digest, copied, err := prepareMigration')
    replace('func replaceMigrationGroup(db *DB, input Input, g codex.Group, uuids []string, adapter codex.Adapter, scan migrationInput, index codex.Group, importedAt string) (int, error) {', 'func replaceMigrationGroup(db *DB, input Input, g codex.Group, uuids []string, adapter codex.Adapter, scan migrationInput, index codex.Group, importedAt string) (int, error) {\n\tdefer perfprobe.Start("replace_group")()')
    replace('\t\terr = tx.QueryRow(`SELECT legacy_rowid', '\t\tperfprobe.Counts["old_lookup_sql"]++\n\t\tendLookup := perfprobe.Start("old_lookup")\n\t\terr = tx.QueryRow(`SELECT legacy_rowid')
    replace('\t\tif err == sql.ErrNoRows {', '\t\tendLookup()\n\t\tif err == sql.ErrNoRows {')
    m.command(['gofmt', '-w', str(path), str(probe)])


def main():
    assert not WORK.exists(), 'Remove only the owned scratch directory before rerunning'
    WORK.mkdir(parents=True)
    m.WORK.mkdir()
    os.environ['GOCACHE'] = str(WORK / 'go-cache')
    os.environ['GOFLAGS'] = '-buildvcs=false'
    copy_source(WORK / 'source')
    m.command(['go', 'build', '-o', str(WORK / 'plain'), './cmd/somniloq'], cwd=WORK / 'source')
    shutil.copytree(WORK / 'source', WORK / 'probe-source')
    instrument(WORK / 'probe-source')
    m.command(['go', 'build', '-o', str(WORK / 'probe'), './cmd/somniloq'], cwd=WORK / 'probe-source')
    report = dict(commit=subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip(),
                  go=subprocess.check_output(['go', 'version'], text=True).strip(),
                  os=subprocess.check_output(['sw_vers'], text=True).strip(), cases=[])
    for case in m.CASES:
        base, details = m.setup(case)
        runs = []
        # Three plain pairs measure variation; three probes bound lookup cost.
        for repetition in range(6):
            measured = repetition >= 3
            for path in base.glob('new.db*'):
                path.unlink()
            for retry in [False, True]:
                result = m.run(WORK / ('probe' if measured else 'plain'), base, details, retry, measured)
                result.update(repetition=repetition)
                runs.append(result)
        report['cases'].append(dict(spec=details, runs=runs))
        m.OUT.write_text(json.dumps(report, indent=2) + '\n')
        print(case[0], 'complete', flush=True)
        if sum(p.stat().st_size for p in WORK.rglob('*') if p.is_file()) > 2 * 1024 ** 3:
            raise RuntimeError('disk budget exceeded')
    print(m.OUT, flush=True)


if __name__ == '__main__':
    main()
