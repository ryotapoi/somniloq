---
observed_at: 2026-10-09
compiled_from_commit: c37343cd083f376852517ebfaa34570690beb31c
baseline_commit: 6cfd9d53a3bf7029e8f034aee5d26e8cc619d17e
---

# migrate 改善の累積効果

一連の改善開始前と現在版を同時期・同入力で比較した。最大会話約43 KiB固定の総量16倍では、初回時間中央値10.80→2.84秒（73.7%短縮）、ピークRSS 98.08→48.81 MiB（50.2%減）。本文64 MiBを16会話に分ける条件は1.22→0.91秒（25.4%短縮）、306.14→62.77 MiB（79.5%減）だった。ただし単一64 MiB会話は0.71→0.84秒（18.3%増）、439.89→465.83 MiB（5.9%増）であり、全条件の時間・RSS改善は成立しない。コード変更・新たな改善の採否は行っていない。

## 比較対象と条件

- 改善開始前: `6cfd9d53a3bf7029e8f034aee5d26e8cc619d17e`。最初の[基準計測](../migration-performance-20261008/README.md)と同じ版。[Git改善前](../git-resolution-performance-20261008/README.md)の `8c441e8` と製品source・go.mod/go.sumに差はない。
- 現在版: `c37343cd083f376852517ebfaa34570690beb31c`、開始時clean。Git解決再利用（`3b0107e`）、会話単位の本文読み込み（`488636f`）、JSONL解析結果共有（`cfb6a21`）を含む。全走査と旧行UUID照合の再評価は実装見送り。現在版と `cfb6a21` に製品source・go.mod/go.sumの差はない。
- macOS 27.0.1 / 26A434、Go 1.27.1 darwin/arm64。既存cacheと同じ作業機。開始時ディスク空き約38 GiB。
- 通常の最適化付き `go build`、両版 `GOFLAGS=-buildvcs=false`、専用の共通 `GOCACHE`。計測器によるsource変更なし。CLI起動から正常終了までを `/usr/bin/time -l` で計測し、build・入力生成・DB検証は計時外。
- 初回は移行先DB未存在、再実行は直前に同じ版で作ったDBと同一snapshotを使用。両版は同じ入力・旧DB・設定・移行先pathを使う。各版の初回前にその条件の `new.db` とsidecarだけを除去した。
- 既存[入力生成・監視・保存検証](../migration-performance-20261008/measure.py)の `setup` / `run` を変更せず再利用。13条件すべてと、[本文メモリ改善cache](../migration-memory-performance-20261009/README.md)の単一64 MiB会話を追加。各条件3組、版順は改善前→現在、現在→改善前、改善前→現在。各版で初回→再実行。計14条件168操作を直列実行し、すべてexit 0。
- 1操作60秒、CLI RSS 1 GiB、専用領域2 GiB。250 msごとに `pgrep` / `ps` で自分のCLIを監視し、時間/RSS超過時はそのprocess groupをSIGTERMで停止する既存手順を使用。ディスク量は条件終了時に検査。計測完了時は約672.6 MiB、予算中断なし。process監視が使えるsandbox外で実行。
- filesystem cacheの冷却・同居作業の制御はしていない。ベンチマークとテストは並行実行していない。先行cacheの過去値を今回の改善率計算に使っていない。

生値・入力寸法・各操作の成功summary・版/反復・harnessと再実行driverのSHA-256は [results.json](results.json)、検証コマンド・終了コード・時間・出力は [gates.json](gates.json)。本文と実ログをartifactに含めない。

## 入力寸法

1 MiB = 1,048,576 bytes。合成JSONLはsession_metaとpayload ID付きassistant本文からなり、非本文追加条件は各本文後に16 KiB paddingのevent_msgを置く。各会話の4発言ごとに旧DBの物理行UUID一致行を置き、他sourceの保持対象も1件置く。path由来UUIDとcwdの長さは今回の専用pathに対応するため、先行cacheとbytesが微差になり得る。今回の両版には同じbytesを渡した。

| 条件ID | 条件 | JSONL MiB | 最大会話 MiB | ファイル/会話 | 発言 |
| --- | --- | ---: | ---: | ---: | ---: |
| base | 基準 | 0.665 | 0.042 | 16/16 | 1,024 |
| bytes4 | 発言bytes 4倍 | 2.165 | 0.135 | 16/16 | 1,024 |
| files4 | 同じ本文を64ファイルに分割 | 0.672 | 0.042 | 64/16 | 1,024 |
| groups4 | 同じ本文を64会話に分割 | 0.672 | 0.011 | 64/64 | 1,024 |
| groups16 | 同じ本文を256会話に分割 | 0.702 | 0.003 | 256/256 | 1,024 |
| cwd16 | 異なるcwd 16個 | 0.665 | 0.042 | 16/16 | 1,024 |
| one_group | 同じ本文を1会話に集約 | 0.665 | 0.665 | 16/1 | 1,024 |
| total4 | 最大会話固定・総量4倍 | 2.661 | 0.042 | 64/64 | 4,096 |
| total16 | 最大会話固定・総量16倍 | 10.655 | 0.042 | 256/256 | 16,384 |
| ignored_large | 大きい非本文record追加 | 16.730 | 1.046 | 16/16 | 1,024 |
| body64MiB | 本文64 MiB・16会話 | 64.165 | 4.010 | 16/16 | 1,024 |
| no_git_256 | cwdなし256会話 | 0.678 | 0.003 | 256/256 | 1,024 |
| no_git_1024 | cwdなし1,024会話 | 0.725 | 0.001 | 1024/1024 | 1,024 |
| giant_group | 単一会話64 MiB | 64.163 | 64.163 | 1/1 | 1,024 |

## 時間・ピークRSS

各セルは計測器なし3回の中央値 [最小–最大]。時間は秒、time表示精度は0.01秒。RSSはMiBでCLI process単位のピークであり、累積allocationや全子processの同時生存量ではない。CPU user/sysも生値に保存した。差は中央値同士の差で、率は `(現在/改善前 - 1) × 100`。外れ値は除外していない。

### 初回

| 条件ID | 改善前 秒 | 現在 秒 | 時間差 秒 (%) | 改善前 RSS MiB | 現在 RSS MiB | RSS差 MiB (%) |
| --- | --- | --- | --- | --- | --- | --- |
| base | 0.62 [0.61–0.89] | 0.13 [0.12–0.46] | -0.49 (-79.0%) | 33.70 [33.41–33.80] | 31.02 [30.94–31.67] | -2.69 (-8.0%) |
| bytes4 | 0.62 [0.62–0.63] | 0.15 [0.14–0.16] | -0.47 (-75.8%) | 39.75 [39.11–39.75] | 31.67 [31.53–32.11] | -8.08 (-20.3%) |
| files4 | 2.08 [2.07–2.09] | 0.13 [0.13–0.13] | -1.95 (-93.8%) | 33.88 [33.83–34.64] | 31.95 [31.83–32.45] | -1.92 (-5.7%) |
| groups4 | 2.13 [2.12–2.16] | 0.17 [0.17–0.18] | -1.96 (-92.0%) | 34.20 [34.16–34.41] | 31.44 [31.20–31.92] | -2.77 (-8.1%) |
| groups16 | 8.31 [8.30–8.43] | 0.41 [0.41–0.42] | -7.90 (-95.1%) | 36.58 [36.36–37.09] | 33.00 [32.86–33.25] | -3.58 (-9.8%) |
| cwd16 | 0.61 [0.60–0.62] | 0.37 [0.36–0.39] | -0.24 (-39.3%) | 33.36 [33.34–33.88] | 31.36 [30.70–31.66] | -2.00 (-6.0%) |
| one_group | 0.58 [0.58–0.59] | 0.11 [0.11–0.11] | -0.47 (-81.0%) | 33.30 [33.28–33.30] | 32.58 [32.55–33.14] | -0.72 (-2.2%) |
| total4 | 2.43 [2.42–2.43] | 0.47 [0.47–0.47] | -1.96 (-80.7%) | 45.95 [45.64–47.53] | 35.27 [35.09–35.53] | -10.69 (-23.3%) |
| total16 | 10.80 [10.74–10.85] | 2.84 [2.83–2.86] | -7.96 (-73.7%) | 98.08 [97.80–98.56] | 48.81 [48.80–48.86] | -49.27 (-50.2%) |
| ignored_large | 0.66 [0.65–0.71] | 0.19 [0.18–0.19] | -0.47 (-71.2%) | 76.20 [75.89–76.50] | 36.25 [35.80–36.72] | -39.95 (-52.4%) |
| body64MiB | 1.22 [1.18–1.26] | 0.91 [0.90–0.97] | -0.31 (-25.4%) | 306.14 [299.78–306.36] | 62.77 [62.00–63.20] | -243.38 (-79.5%) |
| no_git_256 | 0.39 [0.39–0.39] | 0.41 [0.40–0.41] | +0.02 (+5.1%) | 36.12 [34.86–36.31] | 33.41 [32.98–33.55] | -2.72 (-7.5%) |
| no_git_1024 | 2.25 [2.25–2.27] | 2.31 [2.27–2.33] | +0.06 (+2.7%) | 41.30 [41.19–41.77] | 38.02 [37.44–38.28] | -3.28 (-7.9%) |
| giant_group | 0.71 [0.68–0.81] | 0.84 [0.83–0.88] | +0.13 (+18.3%) | 439.89 [439.73–440.20] | 465.83 [465.70–466.05] | +25.94 (+5.9%) |

### 同一snapshot再実行

| 条件ID | 改善前 秒 | 現在 秒 | 時間差 秒 (%) | 改善前 RSS MiB | 現在 RSS MiB | RSS差 MiB (%) |
| --- | --- | --- | --- | --- | --- | --- |
| base | 0.63 [0.59–0.68] | 0.12 [0.12–0.12] | -0.51 (-81.0%) | 33.14 [33.09–33.58] | 31.34 [31.16–31.36] | -1.80 (-5.4%) |
| bytes4 | 0.62 [0.62–0.63] | 0.14 [0.14–0.15] | -0.48 (-77.4%) | 39.56 [39.45–39.61] | 31.88 [31.83–32.25] | -7.69 (-19.4%) |
| files4 | 2.07 [2.07–2.11] | 0.12 [0.12–0.13] | -1.95 (-94.2%) | 34.03 [33.89–34.20] | 31.86 [31.84–31.88] | -2.17 (-6.4%) |
| groups4 | 2.12 [2.11–2.16] | 0.16 [0.16–0.17] | -1.96 (-92.5%) | 34.20 [33.97–34.20] | 31.52 [31.16–32.03] | -2.69 (-7.9%) |
| groups16 | 8.28 [8.25–8.32] | 0.42 [0.41–0.42] | -7.86 (-94.9%) | 36.70 [36.62–37.11] | 32.73 [32.56–33.05] | -3.97 (-10.8%) |
| cwd16 | 0.61 [0.59–0.61] | 0.36 [0.35–0.36] | -0.25 (-41.0%) | 33.28 [33.06–33.59] | 32.14 [31.42–32.14] | -1.14 (-3.4%) |
| one_group | 0.58 [0.57–0.60] | 0.11 [0.11–0.11] | -0.47 (-81.0%) | 33.25 [33.02–33.80] | 33.38 [32.72–33.58] | +0.12 (+0.4%) |
| total4 | 2.44 [2.44–2.53] | 0.48 [0.48–0.49] | -1.96 (-80.3%) | 46.00 [45.78–46.19] | 35.25 [34.81–35.28] | -10.75 (-23.4%) |
| total16 | 11.15 [11.09–11.67] | 3.19 [3.17–3.20] | -7.96 (-71.4%) | 93.73 [92.02–94.11] | 43.73 [43.66–43.86] | -50.00 (-53.3%) |
| ignored_large | 0.64 [0.63–0.66] | 0.18 [0.18–0.18] | -0.46 (-71.9%) | 75.30 [74.19–76.22] | 36.44 [36.09–36.62] | -38.86 (-51.6%) |
| body64MiB | 1.40 [1.39–1.44] | 1.06 [1.05–1.06] | -0.34 (-24.3%) | 301.58 [300.55–301.59] | 56.80 [56.61–58.05] | -244.78 (-81.2%) |
| no_git_256 | 0.38 [0.38–0.39] | 0.40 [0.40–0.41] | +0.02 (+5.3%) | 35.61 [35.59–36.09] | 32.89 [32.69–33.00] | -2.72 (-7.6%) |
| no_git_1024 | 2.26 [2.22–2.28] | 2.29 [2.29–2.31] | +0.03 (+1.3%) | 40.38 [39.67–40.59] | 36.70 [36.36–36.81] | -3.67 (-9.1%) |
| giant_group | 0.89 [0.88–0.91] | 1.05 [1.01–1.06] | +0.16 (+18.0%) | 435.16 [365.83–435.59] | 462.27 [461.94–462.31] | +27.11 (+6.2%) |

## 結果の判断と限界

単一64 MiB会話以外のGitを使う通常repositoryの条件では全体時間が短縮し、総量と複数会話の大きい本文の条件でRSSも減った。個別の[Git](../git-resolution-performance-20261008/README.md)・[本文保持](../migration-memory-performance-20261009/README.md)・[JSONL解析](../jsonl-parse-performance-20261009/README.md)の記録が機構の根拠であり、今回の測定は累積結果だけを確認する。各改善への寄与率は分離していない。`one_group` のRSSは初回-0.72 MiBだが、再実行は+0.12 MiBで範囲も重なるため、両操作に共通するRSS改善を主張しない。

Gitなし256会話は初回0.39→0.41秒、再実行0.38→0.40秒で範囲が重ならなかったが、絶対差は0.02秒と小さく、0.01秒の表示精度と無制御の同居負荷の制約がある。1,024会話は初回2.25→2.31秒、再実行2.26→2.29秒。初回の範囲は端点2.27秒で接し、再実行の範囲は重ならなかった。両条件に時間改善はなく、小さな増加の一般化はしない。全走査の費用は残る。

単一64 MiB会話は初回・再実行とも時間とRSSが増え、両版の反復範囲は重ならない。再実行RSSは改善前に365.83 MiBの低い値もあり、除外せず記録した。過去の本文保持改善cacheでは単一会話RSSが減ったが、その後のJSONL解析共有の比較に単一会話条件は含まれていない。今回はその条件を現在版で測った結果であり、過去の削減率を現在版へ引き継げない。どの変更が増加を生んだかはこの累積A/Bで未分離。修正や追加実験は今回の範囲に含めない。

正常系の合成入力、平坦なdirectory、一つの入力rootでの結果。実ログ・cold cache・GiB級入力・巨大旧DB・多数の失敗診断・並行更新・他filesystem・worktree/submoduleの性能は未計測。最大会話依存のRSS制約は残る。

## 保存検証とgate

168操作すべてで、正常group数・初回コピー有無・旧行削除数/他source保持・保存会話/発言数・本文長と合計・role/membership/timestamp・cursor終端・SQLite integrity、移行元digest不変とsidecar不在を既存harnessで確認した。失敗時保持とsnapshot変更は正常系の性能入力には含めず、両版の既存全package testで確認した。測定前の現在版 `go test -count=1 ./...` はexit 0（3.56秒）。測定後は以下を直列実行し、すべてexit 0。

| 確認 | command | 秒 |
| --- | --- | ---: |
| baseline tests | `go test -count=1 ./...` | 6.428 |
| format | `docs/verification.md の find/gofmt 空確認` | 0.582 |
| tests | `go test -count=1 ./...` | 3.368 |
| vet | `go vet ./...` | 1.537 |
| build | `go build -o bin/somniloq ./cmd/somniloq` | 0.357 |
| licenses install | `go install github.com/google/go-licenses/v2@v2.0.1` | 2.826 |
| notices | `python3 scripts/update-third-party-notices.py --check` | 2.919 |

旧版testは抽出したbefore-sourceで実行。licenses installは専用の一時GOBINを使い、そのPATHでnotice checkを実行。notice checkの既存非Goコードwarningは生ログに残し、終了0を確認した。製品コード・SQL・schema・設定・既存cacheの計測scriptは変更していない。今回保持する変更はこのcacheの記録と生値だけ。

## 再実行

リポジトリrootから実行する。前回processの終了を確認し、既存の `.tmp/migration-cumulative-performance/` がある場合はこの専用領域だけを除去してから実行する。macOSの `time -l`、`pgrep`、`ps` が使える環境が必要。改善前は固定commit、現在版は再実行時の作業ツリーをコピーして測るため、製品sourceが変わっていれば今回と同じ現在版の値ではない。以下のdriverは今回 `.tmp/` に置いて実行したものと同じ。計測器付きbinaryは作らない。

```sh
mkdir -p .tmp/migration-cumulative-performance/before-source
git archive 6cfd9d53a3bf7029e8f034aee5d26e8cc619d17e go.mod go.sum internal cmd | tar -x -C .tmp/migration-cumulative-performance/before-source
cat > .tmp/migration-cumulative-performance/compare.py <<'PY'
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time
sys.dont_write_bytecode = True
repo = Path.cwd().resolve()
work = repo / '.tmp/migration-cumulative-performance'
spec = importlib.util.spec_from_file_location('migration', repo / 'cache/migration-performance-20261008/measure.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
m.WORK = work / 'inputs'
m.OUT = work / 'results.json'
m.CASES += [('giant_group', 1, 1, 1024, 65536, 1, 0)]
assert not m.WORK.exists()
m.WORK.mkdir()
os.environ['GOFLAGS'] = '-buildvcs=false'
os.environ['GOCACHE'] = str(work / 'go-cache')
current = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
for version, commit in [('before', '6cfd9d53a3bf7029e8f034aee5d26e8cc619d17e'), ('after', current)]:
    source = work / (version + '-source')
    if version == 'after':
        source.mkdir()
        for name in ['go.mod', 'go.sum', 'internal', 'cmd']:
            src = repo / name
            shutil.copytree(src, source / name) if src.is_dir() else shutil.copy2(src, source / name)
    m.command(['go', 'build', '-o', str(work / version), './cmd/somniloq'], cwd=source)
report = dict(before_commit='6cfd9d53a3bf7029e8f034aee5d26e8cc619d17e', after_commit=current,
              go=subprocess.check_output(['go', 'version'], text=True).strip(),
              os=subprocess.check_output(['sw_vers'], text=True).strip(),
              started_at=time.strftime('%Y-%m-%dT%H:%M:%S%z'), cases=[])
for case in m.CASES:
    base, details = m.setup(case)
    runs = []
    for repetition in range(3):
        versions = ['before', 'after'] if repetition % 2 == 0 else ['after', 'before']
        for version in versions:
            for path in base.glob('new.db*'):
                path.unlink()
            for retry in [False, True]:
                result = m.run(work / version, base, details, retry, False)
                result.update(version=version, repetition=repetition, exit_code=0, preservation_verified=True)
                runs.append(result)
                print(case[0], repetition, version, 'retry' if retry else 'first', result['wall_s'], round(result['peak_rss_bytes']/1048576, 2), flush=True)
    report['cases'].append(dict(spec=details, runs=runs))
    report['work_bytes'] = sum(p.stat().st_size for p in work.rglob('*') if p.is_file())
    m.OUT.write_text(json.dumps(report, indent=2) + '\n')
    if report['work_bytes'] > 2*1024**3:
        raise RuntimeError('disk budget exceeded')
report['finished_at'] = time.strftime('%Y-%m-%dT%H:%M:%S%z')
m.OUT.write_text(json.dumps(report, indent=2) + '\n')
PY
python3 .tmp/migration-cumulative-performance/compare.py
```

再実行の生値は `.tmp/migration-cumulative-performance/results.json`。今回の [results.json](results.json) を上書きせず、新たな観測として扱う。
