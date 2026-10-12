"""Reproduce the paired Claude owner-comparison measurement from the repository root."""

import json
import os
import pathlib
import shutil
import subprocess
import sys
import time

BASE = pathlib.Path.cwd() / ".tmp/claude-owner-comparison"
COUNT = 128
RECORDS = 128
CHANGED = 96
LIMIT_SECONDS = 30
LIMIT_RSS = 512 * 1024 * 1024


def prepare():
    shutil.rmtree(BASE / "logs", ignore_errors=True)
    for name in ("run.db", "run.db-wal", "run.db-shm", "seed.db"):
        (BASE / name).unlink(missing_ok=True)
    root = BASE / "logs" / "project"
    root.mkdir(parents=True, exist_ok=True)
    for i in range(COUNT):
        path = root / f"session-{i}.jsonl"
        with path.open("w") as stream:
            for j in range(RECORDS):
                stream.write(json.dumps({
                    "type": "user", "uuid": f"m-{j}", "sessionId": f"session-{i}",
                    "timestamp": "2026-01-01T00:00:00Z",
                    "message": {"role": "user", "content": "x" * 256},
                }, separators=(",", ":")) + "\n")
    config = BASE / "config.toml"
    config.write_text(f'db = "{BASE / "run.db"}"\n[[inputs]]\nsource = "claude-code"\nroot = "{root.parent}"\n')
    run("baseline", diagnostic=False)
    shutil.copyfile(BASE / "run.db", BASE / "seed.db")
    for i in range(CHANGED):
        with (root / f"session-{i}.jsonl").open("a") as stream:
            stream.write(json.dumps({
                "type": "assistant", "uuid": "new", "sessionId": f"session-{i}",
                "timestamp": "2026-01-02T00:00:00Z",
                "message": {"role": "assistant", "content": "appended response"},
            }, separators=(",", ":")) + "\n")
    print(json.dumps({"files": COUNT, "records": COUNT * RECORDS, "changed": CHANGED,
                      "input_bytes": sum(p.stat().st_size for p in root.glob("*.jsonl"))}))


def run(binary, diagnostic):
    start = time.perf_counter()
    with (BASE / "stdout").open("w") as out, (BASE / "stderr").open("w") as err:
        proc = subprocess.Popen([str(BASE / binary), "import", "--config", str(BASE / "config.toml")],
                                stdout=out, stderr=err)
        next_rss_check = start
        while True:
            pid, status, usage = os.wait4(proc.pid, os.WNOHANG)
            if pid:
                proc.returncode = os.waitstatus_to_exitcode(status)
                break
            if time.perf_counter() - start > LIMIT_SECONDS:
                proc.kill()
                proc.wait()
                raise RuntimeError("time budget exceeded")
            if time.perf_counter() >= next_rss_check:
                next_rss_check = time.perf_counter() + 0.1
                try:
                    rss = int(subprocess.check_output(["/bin/ps", "-o", "rss=", "-p", str(proc.pid)],
                                                      stderr=subprocess.DEVNULL)) * 1024
                    if rss > LIMIT_RSS:
                        proc.kill()
                        proc.wait()
                        raise RuntimeError("memory budget exceeded")
                except (ValueError, PermissionError, subprocess.CalledProcessError):
                    pass
            time.sleep(0.005)
    if usage.ru_maxrss > LIMIT_RSS:
        raise RuntimeError("memory budget exceeded")
    stdout = (BASE / "stdout").read_text().strip()
    stderr = (BASE / "stderr").read_text().strip()
    if proc.returncode or (stderr and not diagnostic):
        raise RuntimeError((proc.returncode, stdout, stderr))
    result = {"wall": time.perf_counter() - start,
              "cpu": usage.ru_utime + usage.ru_stime, "peak_rss": usage.ru_maxrss,
              "stdout": stdout}
    if diagnostic:
        if not stderr.startswith("CLAUDE_COMPARE "):
            raise RuntimeError(stderr)
        result.update(json.loads(stderr.removeprefix("CLAUDE_COMPARE ")))
    return result


def measure():
    if sum(p.stat().st_size for p in BASE.rglob("*") if p.is_file()) > 1024 ** 3:
        raise RuntimeError("disk budget exceeded")
    for mode in ("plain", "instrumented"):
        for trial in range(-1, 5):
            order = ("baseline", "candidate") if trial % 2 == 0 else ("candidate", "baseline")
            for binary in order:
                for suffix in ("", "-wal", "-shm"):
                    (BASE / ("run.db" + suffix)).unlink(missing_ok=True)
                shutil.copyfile(BASE / "seed.db", BASE / "run.db")
                result = run(binary + ("-instrumented" if mode == "instrumented" else ""),
                             diagnostic=mode == "instrumented")
                expected = (f"Imported {CHANGED} files ({COUNT} scanned, "
                            f"{COUNT - CHANGED} skipped, 0 failed, 0 unparsed lines)")
                if result["stdout"] != expected:
                    raise RuntimeError(result)
                if trial >= 0:
                    print(json.dumps({"mode": mode, "trial": trial, "binary": binary, **result}), flush=True)


def instrument():
    before = ("\tvar compareBefore runtime.MemStats\n\truntime.ReadMemStats(&compareBefore)\n"
              "\tcompareStart := time.Now()\n")
    after = ("\tvar compareAfter runtime.MemStats\n\truntime.ReadMemStats(&compareAfter)\n"
             "\tcompareJSON, _ := json.Marshal(map[string]any{\"compare_seconds\": time.Since(compareStart).Seconds(), "
             "\"compare_alloc_bytes\": compareAfter.TotalAlloc-compareBefore.TotalAlloc, "
             "\"compare_mallocs\": compareAfter.Mallocs-compareBefore.Mallocs})\n"
             "\tfmt.Fprintf(os.Stderr, \"CLAUDE_COMPARE %s\\n\", compareJSON)\n")
    for binary in ("baseline", "candidate"):
        source = BASE / (binary + "-source") / "internal/core/import_claude.go"
        contents = source.read_text()
        if "CLAUDE_COMPARE" in contents:
            raise RuntimeError("restore fresh source archive before instrumenting again")
        contents = contents.replace('import (\n', 'import (\n\t"encoding/json"\n\t"os"\n\t"runtime"\n\t"time"\n', 1)
        marker = "\tif !full && len(changed) > 0 {" if binary == "candidate" else "\tif !full {\n\t\tfor identity := range changed {"
        if marker not in contents:
            raise RuntimeError("comparison entry marker absent")
        contents = contents.replace(marker, before + marker, 1)
        marker = "\ttx, err := db.Begin()"
        if marker not in contents:
            raise RuntimeError("comparison exit marker absent")
        source.write_text(contents.replace(marker, after + marker, 1))
        subprocess.run(["gofmt", "-w", str(source)], check=True)
        env = dict(os.environ, GOCACHE=str(pathlib.Path.cwd() / ".tmp/gocache"))
        subprocess.run(["go", "build", "-o", str(BASE / (binary + "-instrumented")), "./cmd/somniloq"],
                       cwd=source.parents[2], env=env, check=True)


if __name__ == "__main__":
    if sys.argv[1] == "prepare":
        if len(sys.argv) > 2:
            COUNT, RECORDS, CHANGED = map(int, sys.argv[2:5])
        prepare()
    elif sys.argv[1] == "measure":
        if len(sys.argv) > 2:
            COUNT, CHANGED = map(int, sys.argv[2:4])
        measure()
    elif sys.argv[1] == "instrument":
        instrument()
