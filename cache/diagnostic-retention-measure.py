import json, os, pathlib, shutil, subprocess, sys, time
# Generated measurement helper; run from the repository root.
BASE = pathlib.Path.cwd() / '.tmp/diagnostic-performance'
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
    if p.returncode:
        raise RuntimeError((p.returncode, outpath.read_text(), errpath.read_text()))
    if '0 scanned' in outpath.read_text():
        raise RuntimeError('fixture was not scanned')
    return {'wall': round(elapsed, 4), 'cpu': round(usage.ru_utime + usage.ru_stime, 4), 'rss': usage.ru_maxrss, 'stdout': outpath.read_text().strip(), 'stderr': errpath.read_text().strip()}

def fixture(source, shape):
    directory = BASE / (source + '-' + shape)
    root = directory / 'logs' / 'project'
    root.mkdir(parents=True, exist_ok=True)
    for i in range(4):
        if source == 'claude-code':
            record = {'type':'user','uuid':'m-0','sessionId':f'session-{i}','timestamp':'2026-01-01T00:00:00Z','message':{'role':'user','content':'x'*256}}
            lines = json.dumps(record) + '\n'
        else:
            lines = json.dumps({'type':'session_meta','payload':{'id':f'session-{i}'}}) + '\n'
            record = {'type':'response_item','timestamp':'2026-01-01T00:00:00Z','payload':{'type':'message','role':'user','content':[{'type':'input_text','text':'x'*256}]}}
            lines += json.dumps(record) + '\n'
        if shape == 'malformed':
            lines += 'bad\n' * 100000
        else:
            for j in range(1,1000):
                if source == 'claude-code': record['uuid'] = f'm-{j}'
                lines += json.dumps(record) + '\n'
        (root / f'session-{i}.jsonl').write_text(lines)
    config = directory / 'config.toml'
    config.write_text(f'db = "{directory / "run.db"}"\n[[inputs]]\nsource = "{source}"\nroot = "{root.parent}"\n')
    return directory, config

if sys.argv[1] == 'prepare':
    for source in ['claude-code','codex']:
        for shape in ['malformed','clean']:
            directory, config = fixture(source, shape)
            for suffix in ['', '-wal', '-shm']: (directory / ('run.db'+suffix)).unlink(missing_ok=True)
            result = run('baseline', config)
            shutil.copyfile(directory / 'run.db', directory / 'seed.db')
            print(source, shape, result, flush=True)
elif sys.argv[1] == 'measure':
    for source in ['claude-code','codex']:
        for shape in ['malformed','clean']:
            directory = BASE / (source + '-' + shape)
            config = directory / 'config.toml'
            target = directory / 'logs/project/session-0.jsonl'
            original = target.read_bytes()
            for mode in ['fresh','append','unchanged']:
                if mode == 'append': target.write_bytes(original + b'bad\n' * 10)
                for trial in range(-1,3):
                    results = []
                    for binary in (['baseline','candidate'] if trial % 2 == 0 else ['candidate','baseline']):
                        for suffix in ['', '-wal', '-shm']: (directory / ('run.db'+suffix)).unlink(missing_ok=True)
                        if mode != 'fresh': shutil.copyfile(directory / 'seed.db', directory / 'run.db')
                        result = run(binary, config)
                        expected = f'Imported {4 if mode == "fresh" else 1 if mode == "append" else 0} files (4 scanned, {0 if mode == "fresh" else 3 if mode == "append" else 4} skipped, 0 failed, {400000 if shape == "malformed" and mode == "fresh" else 10 if mode == "append" else 0} unparsed lines)'
                        if result['stdout'] != expected: raise RuntimeError(result)
                        results.append(result)
                        if trial >= 0: print(json.dumps({'source':source,'shape':shape,'mode':mode,'trial':trial,'binary':binary, **result}), flush=True)
                    if results[0]['stderr'] != results[1]['stderr']: raise RuntimeError(('diagnostics differ',results))
                target.write_bytes(original)
elif sys.argv[1] == 'allocations':
    import io, tarfile
    commit = '452f7935eb7e2a5d586df1a686874872cfea6f0d'
    archive = subprocess.check_output(['git','archive',commit])
    for binary in ['baseline','candidate']:
        directory = BASE / (binary + '-source')
        directory.mkdir(exist_ok=True)
        with tarfile.open(fileobj=io.BytesIO(archive)) as files: files.extractall(directory, filter='data')
        if binary == 'candidate':
            for name in subprocess.check_output(['git','ls-files','*.go']).decode().splitlines():
                shutil.copyfile(name, directory / name)
        call = 'adapter.BuildGroups(root, paths, "now"' + (', nil)' if binary == 'candidate' else ')')
        test = r'''package ingest_test
import (
 "os"
 "path/filepath"
 "runtime"
 "testing"
 "github.com/ryotapoi/somniloq/internal/ingest/claudecode"
 "github.com/ryotapoi/somniloq/internal/ingest/codex"
)
func BenchmarkDiagnosticRetention(b *testing.B) {
 for _, source := range []string{"claude-code", "codex"} {
  b.Run(source, func(b *testing.B) {
   root := filepath.Join(os.Getenv("DIAGNOSTIC_FIXTURE"), source+"-malformed", "logs")
   paths, _ := filepath.Glob(filepath.Join(root, "project", "*.jsonl"))
   runtime.GC()
   var before, after runtime.MemStats
   runtime.ReadMemStats(&before)
   b.ReportAllocs()
   for i:=0; i<b.N; i++ {
    if source == "claude-code" {
     files, errs, _ := claudecode.NewAdapter(func(string) string {return ""}).BuildSnapshots(root, paths, "now", nil)
     if len(errs)>0 {b.Fatal(errs)}
     runtime.GC(); runtime.ReadMemStats(&after); runtime.KeepAlive(files)
    } else {
     adapter := codex.NewAdapter(func(string) string {return ""})
     groups, errs := CALL
     if len(errs)>0 {b.Fatal(errs)}
     runtime.GC(); runtime.ReadMemStats(&after); runtime.KeepAlive(groups)
    }
   }
   b.ReportMetric(float64(after.HeapAlloc-before.HeapAlloc)/float64(b.N), "retained-B")
  })
 }
}
'''.replace('CALL',call)
        testpath = directory / 'internal/ingest/diagnostic_performance_test.go'
        testpath.write_text(test)
        subprocess.run(['gofmt','-w',str(testpath)],check=True)
    for trial in range(3):
        for binary in (['baseline','candidate'] if trial % 2 == 0 else ['candidate','baseline']):
            env = dict(os.environ, DIAGNOSTIC_FIXTURE=str(BASE))
            result = subprocess.run(['go','test','-run','^$','-bench','BenchmarkDiagnosticRetention','-benchtime','1x','-count=1','./internal/ingest'], cwd=BASE / (binary+'-source'), env=env, capture_output=True, text=True, timeout=30)
            if result.returncode: raise RuntimeError(result.stderr)
            print(binary, trial, result.stdout, flush=True)
