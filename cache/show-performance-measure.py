import hashlib
import json
import os
import pathlib
import subprocess
import time


ROOT = pathlib.Path(".tmp/show-performance")
CASES = {
    "tail": ["--tail", "5"],
    "range": ["--messages", "1996:2000"],
    "limit": ["--limit", "5"],
    "full": [],
}


def wait_with_budget(pid, seconds):
    deadline = time.monotonic() + seconds
    while True:
        completed, status, usage = os.wait4(pid, os.WNOHANG)
        if completed:
            return status, usage
        if time.monotonic() >= deadline:
            try:
                os.kill(pid, 9)
            except ProcessLookupError:
                pass
            os.wait4(pid, 0)
            raise TimeoutError(f"show process {pid} exceeded {seconds} seconds")
        time.sleep(0.01)


ref = json.loads(
    subprocess.check_output(
        [str(ROOT / "baseline"), "search", "--config", str(ROOT / "config.toml"), "--format", "json"],
        timeout=10,
    )
)["items"][0]["ref"]

for name, options in CASES.items():
    for iteration in range(6):
        order = ("baseline", "candidate") if iteration % 2 == 0 else ("candidate", "baseline")
        for side in order:
            destination = ROOT / "output.json"
            with destination.open("wb") as output:
                start = time.perf_counter()
                process = subprocess.Popen(
                    [str(ROOT / side), "show", "--config", str(ROOT / "config.toml"), ref, "--format", "json", *options],
                    stdout=output,
                    stderr=subprocess.PIPE,
                )
                status, usage = wait_with_budget(process.pid, 10)
                wall_ms = (time.perf_counter() - start) * 1000
                process.returncode = os.waitstatus_to_exitcode(status)
                stderr = process.stderr.read()
                if process.returncode:
                    raise RuntimeError((name, side, process.returncode, stderr))
            data = destination.read_bytes()
            result = json.loads(data)
            print(
                json.dumps(
                    {
                        "case": name,
                        "round": iteration,
                        "side": side,
                        "wall_ms": round(wall_ms, 3),
                        "cpu_ms": round((usage.ru_utime + usage.ru_stime) * 1000, 3),
                        "rss_mib": round(usage.ru_maxrss / 1048576, 3),
                        "hash": hashlib.sha256(data).hexdigest(),
                        "total": result["total"],
                        "count": result["count"],
                    }
                ),
                flush=True,
            )
