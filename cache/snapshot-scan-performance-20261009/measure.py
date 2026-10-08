#!/usr/bin/env python3
"""Bounded, repeatable observation of the unchanged migration scan path."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

sys.dont_write_bytecode = True

REPO = Path.cwd().resolve()
WORK = REPO / ".tmp/snapshot-scan-performance"
BASELINE = WORK / "baseline-source"
BASELINE_COMMIT = "3b0107eb2b5f15c396191f667531a8201172c6b9"
spec = importlib.util.spec_from_file_location(
    "migration_measure", REPO / "cache/migration-performance-20261008/measure.py"
)
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
m.WORK = WORK / "inputs"
m.OUT = WORK / "results.json"
m.CASES = [
    ("no_git_256", 256, 1, 4, 512, 0, 0),
    ("no_git_1024", 1024, 1, 1, 512, 0, 0),
]


def main():
    assert BASELINE.exists(), "Extract the recorded baseline commit first"
    assert not m.WORK.exists(), "Remove only this run's owned inputs before rerunning"
    os.environ["GOFLAGS"] = "-buildvcs=false"
    os.environ["GOCACHE"] = str(WORK / "go-cache")
    m.WORK.mkdir(parents=True)
    m.command(["go", "build", "-o", str(WORK / "baseline"), "./cmd/somniloq"], cwd=BASELINE)
    probe_source = WORK / "probe-source"
    shutil.copytree(BASELINE, probe_source)
    m.instrument(probe_source)
    m.command(["go", "build", "-o", str(WORK / "baseline-probe"), "./cmd/somniloq"], cwd=probe_source)
    report = {
        "baseline_commit": BASELINE_COMMIT,
        "run_head": subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip(),
        "go": subprocess.check_output(["go", "version"], text=True).strip(),
        "os": subprocess.check_output(["sw_vers"], text=True).strip(),
        "cases": [],
    }
    for case in m.CASES:
        base, details = m.setup(case)
        runs = []
        for repetition in range(4):
            measured = repetition == 3
            for path in base.glob("new.db*"):
                path.unlink()
            for retry in (False, True):
                binary = WORK / ("baseline-probe" if measured else "baseline")
                result = m.run(binary, base, details, retry, measured)
                result["repetition"] = repetition
                runs.append(result)
        report["cases"].append({"spec": details, "runs": runs})
        m.OUT.write_text(json.dumps(report, indent=2) + "\n")
        print(case[0], "complete", flush=True)
        if sum(path.stat().st_size for path in WORK.rglob("*") if path.is_file()) > 2 * 1024**3:
            raise RuntimeError("disk budget exceeded")
    print(m.OUT, flush=True)


if __name__ == "__main__":
    main()
