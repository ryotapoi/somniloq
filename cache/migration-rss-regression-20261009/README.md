---
observed_at: 2026-10-09
compiled_from_commit: c37343cd083f376852517ebfaa34570690beb31c
---

# 単一巨大会話のRSS増加と改善余地

[累積比較](../migration-cumulative-performance-20261009/README.md)で増えた単一64 MiB会話のRSSには改善余地がある。変更ごとの同条件計測で、本文を会話単位に読み込む版の初回RSS 354.67 MiBに対し、JSONL解析共有後の現在版は466.34 MiBだった。同じ現在binaryでGOGC=100→50を比較すると466.00→290.70 MiB（175.30 MiB、37.6%減）となり、時間中央値は両方0.83秒だった。RSS増加は確認できるが、保持すべき本文が増えたこととは区別する。GCタイミング・解放待ち領域が影響する仮説を支持する対照結果であり、ピークの発生箇所は未特定。

コード変更、既定設定変更、改善の採用は行っていない。今回の範囲は改善余地の判断と原因切り分け。GOGC=50は診断用の一条件で、全入力向けの推奨設定にはしない。

## 条件と生値

既存[入力生成・予算監視・保存検証](../migration-performance-20261008/measure.py)をそのまま使った。単一会話は1ファイル・1,024発言×64 KiB本文。対照は16会話・16ファイル・各64発言×64 KiB本文。同じ本文総量でも最大会話が異なる。これは巨大1行の入力ではない。

macOS 27.0.1 / 26A434、Go 1.27.1 darwin/arm64。全版をcommitから抽出し、通常go build、GOFLAGS=-buildvcs=false、既存の専用GOCACHEでbuild。入力生成・build・保存検証は計時外。time -lでCLI起動から完了までの時間とprocess単位のピークRSSを測った。filesystem cacheと同居負荷は無制御、他のベンチマークとtestは並行しない。初回は移行先DB未存在、再実行はその版/設定で直前に作ったDBと同じsnapshot。各初回前に当該new.dbとsidecarだけを除去した。

4版×2条件×3反復×初回/再実行=48操作。版順は古い→新しい、新しい→古い、古い→新しい。次に現在版・単一会話でGOGC=100/50を交互3組、初回/再実行の12操作を実行。元のGOGC/GOMEMLIMITはどちらも未設定で、環境変数は測定driverとその子processにのみ設定した。各操作60秒、CLI RSS 1 GiB、専用領域2 GiB、250 msごとのpgrep/ps監視と条件終了時の容量確認を再利用。予算中断なし。新規領域の容量はresults.json、Go build cacheは先行計測の専用領域を再利用した。

[変更ごとの生値](results.json)と[GC対照の生値](gc-results.json)に入力寸法、commit、各反復のwall/user/sys/RSSと成功summaryを保存。表は各3回の中央値 [最小–最大]、秒とMiB（1 MiB=1,048,576 bytes）。外れ値は除外しない。時間の表示精度は0.01秒。RSSは累積allocationではない。

## 変更ごとの比較

版の順序は、改善前 `6cfd9d5`、Git解決再利用 `3b0107e`、会話単位の本文読み込み `488636f`、現在 `c37343c`。現在とJSONL解析共有commit `cfb6a21` の製品sourceに差はない。

### giant_group

| 版 | 初回 秒 | 初回RSS MiB | 再実行 秒 | 再実行RSS MiB |
| --- | --- | --- | --- | --- |
| original | 0.73 [0.71–1.17] | 440.08 [440.08–440.09] | 0.90 [0.90–0.95] | 435.58 [435.55–435.70] |
| git_reuse | 0.76 [0.69–1.08] | 439.86 [439.84–440.06] | 0.88 [0.87–0.93] | 435.45 [434.84–435.48] |
| body_loading | 0.98 [0.90–1.25] | 354.67 [354.50–354.69] | 1.13 [1.12–1.48] | 365.41 [363.41–366.19] |
| current | 0.97 [0.85–1.36] | 466.34 [465.78–466.95] | 1.22 [1.06–1.26] | 462.03 [461.97–462.08] |

### body64MiB

| 版 | 初回 秒 | 初回RSS MiB | 再実行 秒 | 再実行RSS MiB |
| --- | --- | --- | --- | --- |
| original | 1.14 [1.13–1.15] | 298.39 [297.84–306.44] | 1.37 [1.37–1.38] | 301.84 [300.56–301.94] |
| git_reuse | 0.68 [0.67–0.70] | 300.88 [299.55–300.97] | 0.89 [0.87–0.89] | 295.67 [294.53–295.72] |
| body_loading | 0.96 [0.93–0.99] | 63.80 [61.45–64.25] | 1.13 [1.12–1.18] | 59.39 [59.20–59.72] |
| current | 0.88 [0.87–0.89] | 61.62 [61.22–65.42] | 1.05 [1.03–1.06] | 58.27 [57.11–60.56] |

単一会話のRSSは本文読み込み版→現在で初回+111.67 MiB、再実行+96.62 MiB。RSSの反復範囲は重ならない。時間は各版の範囲が広く重なり、この4版比較の小差から時間の増加要因を決めない。16会話の現在版RSSは初回61.62 MiB、再実行58.27 MiBで、分散入力の改善を維持している。

## GC対照

| GOGC | 操作 | 時間 秒 | CPU user+sys 秒 | RSS MiB |
| ---: | --- | --- | --- | --- |
| 100 | 初回 | 0.83 [0.81–0.83] | 0.83 [0.81–0.83] | 466.00 [465.42–466.59] |
| 100 | 再実行 | 1.05 [1.03–1.08] | 1.02 [1.00–1.06] | 461.86 [461.38–462.16] |
| 50 | 初回 | 0.83 [0.82–0.85] | 0.84 [0.84–0.88] | 290.70 [290.53–291.25] |
| 50 | 再実行 | 1.06 [1.02–1.07] | 1.05 [1.00–1.06] | 285.91 [285.84–286.12] |

GOGC=50で再実行RSSも461.86→285.91 MiB（38.1%減）、時間は1.05→1.06秒、CPUは1.02→1.05秒。時間・CPUの範囲は重なり、速度差を確定しない。この対照はGCに敏感なRSSであることを示すが、どの割当がピークを作るか、全入力に同じ設定が適切かは示さない。runtime設定の一律変更や強制GCの追加は採用しない。

## コード上の削減候補

fresh Astraによる読み取り専用調査と親agentの実体確認。以下の確保・保持は確認した事実だが、各変更による実際のRSS削減量は未計測。

1. **重複判定の全文JSON署名を作らない。** [group.go](../../internal/ingest/codex/group.go) の227–239行はRole/Blocks/TimestampをJSON化してpayload ID→全文署名mapに保存する。今回の入力は全payload IDが一意なので約64 MiBの本文表現が蓄積する。既に保持している代表メッセージとの正確比較に置き換える余地がある。本文一致だけでなくrole・timestamp・block境界、重複排除と衝突検出、番号・context除外を保つ必要がある。hashだけの比較は完全一致の保証を変えるため候補にしない。通常importとも共有する部分である。
2. **migrationの本文passで元ファイルbytesを保持しない。** 同group.goの109、165–166行は全ファイルをReadFileしてFileReport.Dataに保持する。migration側はPath/Hash/物理行等を使い、Dataを参照していない。通常importのDiagnostics（255–262行）ではDataが必要なので、migration側の用途に限って保持を省く候補。読み取り時点の確保は残り、手放すだけで必ずRSSが減るとは限らない。
3. **snapshot hashをstreamingで計算する。** [migrate.go](../../internal/core/migrate.go) の243–260行はsnapshot確認時にもReadFileで全文を確保する。hashをbuffer付き読み込みで計算すれば、本文group生存中の追加64 MiB bufferを避ける候補。置換前/commit前のhash確認、file集合確認、読み取り失敗の扱いを省かない。

上記はいずれも必要な本文や保存検証の省略ではなく、既存の複製・一時確保を減らす案。まず全文署名とmigration用Dataの保持を測り、一つずつ比較する価値がある。index passで本文をparse/normalizeして捨てる仕事の削減も候補だが、strict拒否・所属証拠への影響が広く、最初の試行にはしない。単一text blockのBlocks/Contentが必ず別の本文コピーを持つとはいえず、重複量として足さない。

## 検証と未確認

60操作すべてexit 0。既存harnessが正常group数・copy有無・旧行削除/他source保持・保存会話/発言数・本文長/総量・role/membership/timestamp・cursor終端・SQLite integrity・移行元digest不変・sidecar不在を確認した。コード編集はなく、変更前のfixture/test証拠は先行cacheを再利用し、追加計測後に共通gateも再実行した。[gates.json](gates.json)にコマンド・終了コード・時間・出力を残す。通常importの診断や失敗時保持を変更した場合の追加検証は、候補を実装するときに必要。

heap profile、GC回数、最大RSS発生段階、候補修正の効果、他入力でのGOGC=50の代償は未確認。実ログ・cold cache・GiB級・大量の短いrecord・並行更新の性能に外挿しない。

## 再実行

自分のprocess終了を確認し、前回の専用 `.tmp/migration-rss-regression` があればその領域だけ除去してから、リポジトリrootで実行する。`pgrep`/`ps`が使える環境が必要。Go cacheは先行計測の専用領域を再利用する。以下のdriverを今回の `.tmp/` に保存して実行した。製品sourceはcommitから抽出するだけで編集しない。

```sh
mkdir -p .tmp/migration-rss-regression
cat > .tmp/migration-rss-regression/measure.py <<'PY'
import importlib.util, io, json, os, subprocess, sys, tarfile, time
from pathlib import Path
sys.dont_write_bytecode=True
repo=Path.cwd().resolve(); work=repo/'.tmp/migration-rss-regression'
spec=importlib.util.spec_from_file_location('baseline',repo/'cache/migration-performance-20261008/measure.py')
m=importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
m.WORK=work/'inputs'; assert not m.WORK.exists(); m.WORK.mkdir()
m.CASES=[('giant_group',1,1,1024,65536,1,0),('body64MiB',16,1,64,65536,1,0)]
os.environ['GOCACHE']=str(repo/'.tmp/migration-cumulative-performance/go-cache')
os.environ['GOFLAGS']='-buildvcs=false'
versions=[('original','6cfd9d5'),('git_reuse','3b0107e'),('body_loading','488636f'),('current','c37343c')]
commits={}
for version,revision in versions:
 commit=subprocess.check_output(['git','rev-parse',revision],text=True).strip(); commits[version]=commit
 source=work/(version+'-source'); source.mkdir()
 archive=subprocess.check_output(['git','archive',commit,'go.mod','go.sum','internal','cmd'])
 with tarfile.open(fileobj=io.BytesIO(archive)) as tar: tar.extractall(source,filter='data')
 m.command(['go','build','-o',str(work/version),'./cmd/somniloq'],cwd=source)
report=dict(commits=commits,go=subprocess.check_output(['go','version'],text=True).strip(),os=subprocess.check_output(['sw_vers'],text=True).strip(),started_at=time.strftime('%Y-%m-%dT%H:%M:%S%z'),cases=[])
for case in m.CASES:
 base,details=m.setup(case); runs=[]
 for repetition in range(3):
  order=versions if repetition%2==0 else list(reversed(versions))
  for version,_ in order:
   for path in base.glob('new.db*'): path.unlink()
   for retry in [False,True]:
    result=m.run(work/version,base,details,retry,False)
    result.update(version=version,repetition=repetition,exit_code=0,preservation_verified=True); runs.append(result)
    print(case[0],repetition,version,'retry' if retry else 'first',result['wall_s'],round(result['peak_rss_bytes']/1048576,2),flush=True)
 report['cases'].append(dict(spec=details,runs=runs))
 report['work_bytes']=sum(p.stat().st_size for p in work.rglob('*') if p.is_file())
 (work/'results.json').write_text(json.dumps(report,indent=2)+'\n')
 if report['work_bytes']>2*1024**3: raise RuntimeError('disk budget exceeded')
report['finished_at']=time.strftime('%Y-%m-%dT%H:%M:%S%z'); (work/'results.json').write_text(json.dumps(report,indent=2)+'\n')
PY
python3 .tmp/migration-rss-regression/measure.py
cat > .tmp/migration-rss-regression/gc.py <<'PY'
import importlib.util,json,os,sys
from pathlib import Path
sys.dont_write_bytecode=True
repo=Path.cwd().resolve(); work=repo/'.tmp/migration-rss-regression'
s=importlib.util.spec_from_file_location('baseline',repo/'cache/migration-performance-20261008/measure.py'); m=importlib.util.module_from_spec(s); s.loader.exec_module(m)
m.CASES=[('giant_group',1,1,1024,65536,1,0)]
details=json.loads((work/'results.json').read_text())['cases'][0]['spec']; base=work/'inputs/giant_group'
report=dict(commit='c37343cd083f376852517ebfaa34570690beb31c',spec=details,initial_environment={k:os.environ.get(k) for k in ['GOGC','GOMEMLIMIT']},runs=[])
for repetition in range(3):
 for value in ([100,50] if repetition%2==0 else [50,100]):
  os.environ['GOGC']=str(value)
  for path in base.glob('new.db*'): path.unlink()
  for retry in [False,True]:
   result=m.run(work/'current',base,details,retry,False); result.update(gogc=value,repetition=repetition,exit_code=0,preservation_verified=True); report['runs'].append(result)
   print(repetition,value,'retry' if retry else 'first',result['wall_s'],round(result['peak_rss_bytes']/1048576,2),flush=True)
   (work/'gc-results.json').write_text(json.dumps(report,indent=2)+'\n')
PY
python3 .tmp/migration-rss-regression/gc.py
```
