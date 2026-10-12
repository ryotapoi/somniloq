import base64
import hashlib
import json
import os
import pathlib
import sqlite3
import statistics
import subprocess
import time

ROOT = pathlib.Path(".tmp/show-relations").resolve()

def identity_ref(key, identity):
    encoded = base64.urlsafe_b64encode(identity.encode()).decode().rstrip('=')
    return f'slq1:{key}:codex:{encoded}'

def seed():
    logs = ROOT / 'logs'
    logs.mkdir(exist_ok=True)
    with (logs / 'target.jsonl').open('w') as f:
        f.write(json.dumps({'type': 'session_meta', 'timestamp': '2026-10-01T00:00:00Z', 'payload': {'id': 'target', 'cwd': '/fixtures/project'}}) + '\n')
        for i in range(4):
            f.write(json.dumps({'type': 'response_item', 'timestamp': '2026-10-01T00:01:00Z', 'payload': {'type': 'message', 'role': 'user', 'content': [{'type': 'input_text', 'text': 'target ' + str(i)}]}}) + '\n')
    for count in (0, 1000, 10000):
        dbpath = ROOT / f'{count}.db'
        dbpath.unlink(missing_ok=True)
        config = ROOT / f'{count}.toml'
        config.write_text(f"db = '{dbpath}'\n[[inputs]]\nsource = 'codex'\nroot = '{logs}'\n")
        subprocess.run([str(ROOT / 'baseline'), 'import', '--config', str(config)], check=True, stdout=subprocess.DEVNULL)
        db = sqlite3.connect(dbpath)
        session_columns = [r[1] for r in db.execute('pragma table_info(sessions)')]
        original = dict(zip(session_columns, db.execute('select * from sessions').fetchone()))
        key = db.execute('select input_key from inputs').fetchone()[0]
        ref = identity_ref(key, original['identity'])
        for owner in range(64):
            s = dict(original)
            s.update(session_id=f'unrelated-{owner}', identity=json.dumps([f'unrelated-{owner}'], separators=(',', ':')))
            db.execute('insert into sessions values (' + ','.join('?' for _ in session_columns) + ')', list(s.values()))
        content = 'x' * 4096
        for i in range(count):
            owner = i % 64
            db.execute('insert into messages (input_id,uuid,source,session_id,identity,role,content,blocks_json,timestamp,number) values (?,?,?,?,?,?,?,?,?,?)',
                       (original['input_id'], f'unrelated-message-{i}', 'codex', f'unrelated-{owner}', json.dumps([f'unrelated-{owner}'], separators=(',', ':')), 'user', content, json.dumps([content]), '2026-10-01T00:01:00Z', i // 64 + 1))
        db.commit()
        db.close()
        (ROOT / f'{count}.ref').write_text(ref)

def run(side, config, args):
    destination = ROOT / 'output.json'
    with destination.open('wb') as out, (ROOT / 'stderr.txt').open('wb') as err:
        start = time.perf_counter()
        p = subprocess.Popen([str(ROOT / side), 'show', '--config', str(config), '--format', 'json', *args], stdout=out, stderr=err)
        deadline = start + 10
        while True:
            done, status, usage = os.wait4(p.pid, os.WNOHANG)
            if done:
                break
            if time.perf_counter() > deadline:
                p.kill()
                os.wait4(p.pid, 0)
                raise TimeoutError(args)
            time.sleep(0.001)
        wall = (time.perf_counter() - start) * 1000
        p.returncode = os.waitstatus_to_exitcode(status)
    data = destination.read_bytes()
    page = json.loads(data) if p.returncode == 0 else {}
    return dict(total=page.get("total"), count=page.get("count"), exit=p.returncode, stdout=hashlib.sha256(data).hexdigest(), stderr=(ROOT / 'stderr.txt').read_text(), wall_ms=wall, cpu_ms=(usage.ru_utime + usage.ru_stime)*1000, rss_mib=usage.ru_maxrss/1048576)

def measure():
    results = []
    for count in (0, 1000, 10000):
        config = ROOT / f'{count}.toml'
        ref = (ROOT / f'{count}.ref').read_text()
        for name, args in [('full', [ref]), ('limit', ['--limit', '1', ref]), ('multiple', [ref, identity_ref(ref.split(':')[1], '["unrelated-0"]')]), ('invalid', ['invalid-ref'])]:
            for iteration in range(8):
                pair = []
                for side in (('baseline', 'candidate') if iteration % 2 == 0 else ('candidate', 'baseline')):
                    r = dict(unrelated_messages=count, case=name, iteration=iteration, side=side, **run(side, config, args))
                    assert r["exit"] == (2 if name == "invalid" else 0), r
                    assert r["rss_mib"] < 512, r
                    if name != "invalid":
                        total = 4 + ((count + 63) // 64 if name == "multiple" else 0)
                        assert r["total"] == total and r["count"] == (1 if name == "limit" else total), r
                    results.append(r)
                    pair.append(r)
                assert (pair[0]['exit'], pair[0]['stdout'], pair[0]['stderr']) == (pair[1]['exit'], pair[1]['stdout'], pair[1]['stderr']), pair
    (ROOT / 'paired.json').write_text(json.dumps(results, indent=2))
    for count in (0, 1000, 10000):
        for case in ('full', 'limit', 'multiple', 'invalid'):
            summary = {'unrelated_messages': count, 'case': case}
            for side in ('baseline', 'candidate'):
                rs = [r for r in results if r['unrelated_messages'] == count and r['case'] == case and r['side'] == side]
                summary[side] = {metric: [round(statistics.median(r[metric] for r in rs), 3), round(min(r[metric] for r in rs), 3), round(max(r[metric] for r in rs), 3)] for metric in ('wall_ms', 'cpu_ms', 'rss_mib')}
            print(json.dumps(summary))

if __name__ == '__main__':
    import sys
    if sys.argv[1] == 'seed':
        seed()
    else:
        measure()
