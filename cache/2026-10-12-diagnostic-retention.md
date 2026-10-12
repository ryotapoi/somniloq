---
generated: 2026-10-12
baseline_commit: 452f7935eb7e2a5d586df1a686874872cfea6f0d
candidate: この記録と同じ差分の snapshot_diagnostics.go、Claude snapshot、Codex owner group と core import
---

# Claude・Codex 解析失敗の診断保持量の計測

採用: cursor の診断開始位置を解析前に確定し、全失敗数・最初の失敗・新規失敗の先頭5件だけを逐次集計する。失敗ごとの offset と error の配列を保持せず、保持しない診断の path・物理行付き error を生成しない。Codex の同一 error の二重生成も除去する。全行の解析、履歴・未完成末尾の cursor、厳密移行の最初の失敗と拒否は維持する。通常 import の診断には事前に捕捉した expected cursor を使い、full と migration は cursor なしで集計する。

仮の成功条件: malformed 入力のピーク RSS がばらつきを超えて減り、正常入力の待ち時間に実質的な悪化を加えず、既存の総数・公開診断5件・新規失敗・末尾再処理・厳密移行の契約を保つ。機能保証を省く案と、失敗ごとの offset だけを残す案は採らない。新しい cache・再読込・同期管理は不要。

## 条件

macOS Darwin 27.0.0 arm64、Apple M4 Pro、Go 1.27.1。通常の `go build`、runtime 既定。入力と SQLite は `.tmp/diagnostic-performance/` に隔離し、実ログ・実DBは使わない。baseline は上記 commit の作業開始時の clean source から保存した binary、candidate は最終変更から build した binary。

各 source 4ファイル。malformed は各ファイルに正常本文1件（Codex は先頭 owner metadata も置く）と `bad\n` 100,000行、合計400,000失敗。clean は各ファイルに256文字の本文1,000件、合計4,000 records。fresh は空DB、append はbaselineで初回取込したseed DBの複製から1ファイルに `bad\n` 10行を追加、unchanged はseed複製と元入力。生成・DB複製は計時外。CLI起動から終了までを wall、`wait4` の user+system を CPU、Darwin `ru_maxrss` を RSS とする。

各条件で両版1回 warmup、その後A/B・B/Aの交互順で3対、直列実行。上限は子process30秒、RSS512MiB、一時物1GiB。100ms間隔のpsでRSSを監視し、終了時peakも検査する。最大実測RSSはwarmupを含め512MiB未満、一時物は48MiB。stdout の imported/scanned/skipped/failed/unparsed と exit 0 を検査し、stderrを対ごとに完全一致させる。fresh malformedは400,000失敗、appendは10失敗、unchangedは0失敗。公開stderrは必要な先頭5件だけ、正常／unchangedは空。

## 最終対比較

中央値（最小–最大）、各版3回。

| source・入力・状態 | baseline RSS MiB | candidate RSS MiB | baseline wall 秒 | candidate wall 秒 | baseline CPU 秒 | candidate CPU 秒 |
|---|---:|---:|---:|---:|---:|---:|
| claude-code・malformed・fresh | 200.42 (196.81–200.86) | 19.98 (19.88–20.12) | 0.1983 (0.1980–0.2014) | 0.1476 (0.1463–0.1540) | 0.2608 (0.2578–0.2674) | 0.2080 (0.2070–0.2157) |
| claude-code・malformed・append | 200.25 (196.14–200.55) | 19.44 (19.42–19.44) | 0.1987 (0.1972–0.2001) | 0.1453 (0.1448–0.1471) | 0.2642 (0.2594–0.2642) | 0.2045 (0.2042–0.2115) |
| claude-code・malformed・unchanged | 200.25 (195.69–201.27) | 19.42 (19.34–19.92) | 0.1970 (0.1966–0.1985) | 0.1433 (0.1428–0.1436) | 0.2619 (0.2587–0.2653) | 0.2038 (0.2028–0.2074) |
| claude-code・clean・fresh | 34.16 (33.56–34.30) | 34.12 (34.03–34.23) | 0.2323 (0.2320–0.2356) | 0.2365 (0.2359–0.2374) | 0.2414 (0.2411–0.2446) | 0.2460 (0.2444–0.2466) |
| claude-code・clean・append | 23.20 (23.14–23.53) | 23.39 (23.00–23.53) | 0.0263 (0.0260–0.0267) | 0.0263 (0.0257–0.0271) | 0.0317 (0.0305–0.0325) | 0.0305 (0.0303–0.0325) |
| claude-code・clean・unchanged | 21.36 (20.66–21.70) | 20.75 (20.69–22.00) | 0.0225 (0.0221–0.0227) | 0.0229 (0.0226–0.0230) | 0.0273 (0.0262–0.0279) | 0.0268 (0.0264–0.0289) |
| codex・malformed・fresh | 83.52 (82.27–85.97) | 20.36 (19.91–20.44) | 0.2439 (0.2361–0.2478) | 0.1314 (0.1312–0.1316) | 0.3234 (0.3148–0.3278) | 0.1764 (0.1756–0.1769) |
| codex・malformed・append | 60.62 (60.58–62.61) | 19.64 (19.55–19.92) | 0.0678 (0.0676–0.0681) | 0.0404 (0.0399–0.0404) | 0.0858 (0.0855–0.0883) | 0.0504 (0.0504–0.0513) |
| codex・malformed・unchanged | 13.94 (13.59–14.00) | 14.03 (13.98–14.05) | 0.0087 (0.0086–0.0087) | 0.0087 (0.0085–0.0090) | 0.0078 (0.0077–0.0079) | 0.0077 (0.0076–0.0080) |
| codex・clean・fresh | 31.52 (31.39–32.30) | 31.53 (31.47–31.77) | 0.2418 (0.2413–0.2427) | 0.2419 (0.2407–0.2420) | 0.2515 (0.2508–0.2517) | 0.2504 (0.2502–0.2510) |
| codex・clean・append | 30.31 (29.72–30.36) | 30.34 (30.30–30.44) | 0.0738 (0.0726–0.0757) | 0.0733 (0.0730–0.0741) | 0.0753 (0.0741–0.0764) | 0.0741 (0.0739–0.0752) |
| codex・clean・unchanged | 13.88 (13.56–13.92) | 13.83 (13.72–13.91) | 0.0091 (0.0088–0.0101) | 0.0089 (0.0087–0.0099) | 0.0080 (0.0080–0.0083) | 0.0080 (0.0080–0.0083) |

malformed fresh のRSSはClaude90.0%、Codex75.6%減、wallは25.6%／46.1%減。malformed appendもRSSは90.3%／67.6%減、wallは26.9%／40.4%減。分布が重ならず、全体操作の約27–113msと41–180MiBの削減を確認した。Claude unchangedも解析するため効き、Codex unchangedは解析をskipするため効果なし。

代償: clean Claude freshのwall中央値は4.2ms（1.8%）、CPUは4.6ms増。この3対ではwallの範囲がわずかに離れたが、前の確認runでは同条件の差は1.2–2.7msで範囲が重なった。正常入力の速度改善は成果とせず、この絶対待ち時間の増加は許容する。clean append/unchangedとCodex cleanの差は小さく、範囲も重なる。入力規模を増やして正常時の増加を外挿する根拠はない。

## 原因の補助確認

同じmalformed入力を一時sourceに置いた `go test -bench` で各版3回、`-benchtime 1x`。snapshot構築後にGCし、`runtime.KeepAlive` でsnapshotを生存させた状態のHeapAlloc増分と累積allocationsを測る。強制GCを含む局所時間を最終wall値には使わない。各ファイルの先頭に正常本文があり、snapshot全体を保持したまま測る。

| source | baseline生存heap MiB | candidate生存heap MiB | baseline累積割当 MiB | candidate累積割当 MiB | baseline allocations | candidate allocations |
|---|---:|---:|---:|---:|---:|---:|
| Claude | 128.918 (128.917–128.918) | .041 (.040–.041) | 386.506 (386.497–386.509) | 240.083 (240.081–240.083) | 6,399,731 (6,399,699–6,399,733) | 4,800,750 (4,800,750–4,800,756) |
| Codex | 124.908 (124.907–124.909) | 1.554 (1.553–1.555) | 408.426 (408.425–408.427) | 179.003 (178.997–179.005) | 7,998,653 (7,998,645–7,998,654) | 4,800,675 (4,800,653–4,800,679) |

失敗配列とwrapped errorが生存量の主因だったことを支持する。累積割当はなお各行のJSON parse error・line bufferに比例する。Codex rawとnormalized履歴、Claude normalized履歴、一個の巨大行／ファイルの保持は今回の削減対象外。大量の長いmalformed payload、正常な巨大履歴、cold disk、他OS/機種は未計測。保存する意味を変えないため既存DBの `--full` は不要。元ログの自動削除は行わない。

## 正しさ・gate

変更前の `go test -count=1 ./internal/ingest/... ./internal/core` は exit 0（core 4.792秒）。候補の同commandも成功（core 4.906秒）。unitでprefix変更・境界・前回未終端行・先頭5件・上限を超える旧失敗後の新規失敗と総失敗数を確認する。全体testには `TestImport_RetriesUnfinishedFinalLine`、`TestMigrationGroupsRejectUnsafeOwnershipWithoutChangingImportContract`、`TestMigrationStrictDiagnosticRetainsPhysicalLineNumber`、旧履歴と失敗時保持のmigration fixtureが含まれ、Cursorの既存testも実行する。

最終source変更後の共通gate:

- `format check`: exit 0、0.117秒。
- `go test -count=1 ./...`: exit 0、5.269秒。
- `go vet ./...`: exit 0、0.214秒。
- `go build -o bin/somniloq ./cmd/somniloq`: exit 0、0.294秒。
- `go install github.com/google/go-licenses/v2@v2.0.1`: exit 0、0.820秒。
- `python3 scripts/update-third-party-notices.py --check`: exit 0、2.567秒。

Go変更はgoimportsで整理した。notice checkは既存依存の非Go assemblyを検査できない警告を出したがexit 0。途中の全testはBuildGroupsのsignature変更に追従していないfunction値caller2箇所のcompile errorでexit 1（5.284秒）。callerを更新して上記最終gateを通した。失敗runに残留test processはない。

## 再実行

[計測helper](diagnostic-retention-measure.py) は生成時点の再現用で、恒久性能gateではない。repository rootで実行する。psとGo cacheへのアクセスがsandboxで拒否される場合は計測commandを権限拡張する。

```sh
mkdir -p .tmp/diagnostic-performance/baseline-source
git archive 452f7935eb7e2a5d586df1a686874872cfea6f0d | tar -x -C .tmp/diagnostic-performance/baseline-source
(cd .tmp/diagnostic-performance/baseline-source && go build -o ../baseline ./cmd/somniloq)
go build -o .tmp/diagnostic-performance/candidate ./cmd/somniloq
python3 cache/diagnostic-retention-measure.py prepare
python3 cache/diagnostic-retention-measure.py measure > .tmp/diagnostic-performance/results.jsonl
python3 cache/diagnostic-retention-measure.py allocations > .tmp/diagnostic-performance/allocations.txt
```

JSONLのsource/shape/mode/binaryごとにwall/cpu/rssの中央値・最小・最大を取る。RSSはbytesを1,048,576で割る。helperは元commitを一時sourceへ展開し、candidate側だけ現在のtracked Go filesで更新し、測定用benchmarkを一時生成する。生存heapと累積割当の列はbenchmarkのretained-B/B/op、allocationsはallocs/op。上限超過や件数／診断不一致は試験失敗にする。初期の入力配置誤りで0 scannedとなった予備実行は採否に含めない。
