---
generated: 2026-10-12
baseline_commit: 5a684691ce521d754c406ffe2637a7e41a9469ce
candidate: この記録と同じ差分の internal/core/import_claude.go と internal/ingest/claudecode/snapshot.go
---

# Claude 差分 import の raw 保持量の計測

採用: cursor に基づく既存の診断集計を各ファイルの解析直後に行い、snapshot に raw bytes を保持しない。全ファイルの解析・正規化・関係証拠は引き続き読む。cache や追加の読み直し経路は設けない。対象は [backlog の全履歴同時保持の検討](../backlog/backlog.md) 一件であり、失敗診断の保持量や会話比較の反復走査は変更しない。

## 条件と予算

- macOS Darwin 27.0.0 arm64、M4 MacBook Pro、Go 1.27.1。`go build` の通常設定、Go runtime 設定は既定。
- 合成した各 32 ファイル × 128 user records。text は各 record の string 本文が 8,192 bytes、tools は同量の tool_result content（保存対象の text は空）。raw 総量は text 34,082,112 bytes、tools 34,324,352 bytes。それぞれ 4,096 records。root は `.tmp/claude-performance/<shape>/logs/project/`。実ログは使わない。
- unchanged は初回取込済み SQLite の複製。append はその同じ seed から一ファイルだけに 172 bytes の assistant 本文を一行追記する。DB 初期化・複製と入力生成は計時外。CLI の起動から終了までを測る。
- 同居テスト・build を止めて直列実行。各条件で両版を一回ずつ warmup し、その後 A/B・B/A の順を交互にした各版 5 回。各回 seed DB に戻す。stdout の scanned/imported/skipped、stderr 空、終了 0 を検査する。
- 一 process 30 秒、RSS 512 MiB、作業 directory 1 GiB を上限とし、超過時は当該子を停止／試験失敗とする。100 ms 間隔の `ps` で RSS を監視し、終了時 `wait4` の peak も検査する。最大実測 RSS は warmup を含め 512 MiB 未満、作業一時物は 221 MiB。wall は monotonic clock、CPU と peak RSS は子の `wait4`（Darwin の RSS 単位は bytes）。

## 結果

中央値（最小–最大）。RSS は MiB、wall/CPU は秒。

| 入力・状態 | baseline RSS | candidate RSS | RSS 減 | baseline wall | candidate wall | baseline CPU | candidate CPU |
|---|---:|---:|---:|---:|---:|---:|---:|
| text・unchanged | 114.25 (113.05–117.28) | 74.05 (72.38–75.64) | 35.2% | .1704 (.1557–.1762) | .1589 (.1557–.1739) | .1815 (.1660–.1879) | .1788 (.1742–.1908) |
| text・append | 124.19 (121.77–130.83) | 82.84 (82.22–88.09) | 33.3% | .1819 (.1778–.1991) | .1805 (.1773–.1996) | .1930 (.1890–.2094) | .1995 (.1967–.2191) |
| tools・unchanged | 73.02 (70.53–74.88) | 24.25 (23.53–24.42) | 66.8% | .1776 (.1754–.1886) | .1784 (.1768–.1803) | .1921 (.1889–.2019) | .2249 (.2225–.2253) |
| tools・append | 79.03 (78.86–79.09) | 28.73 (28.19–29.08) | 63.6% | .1820 (.1810–.1889) | .1874 (.1840–.2147) | .1953 (.1941–.2007) | .2324 (.2300–.2619) |

全回、unchanged は `Imported 0 files (32 scanned, 32 skipped, 0 failed, 0 unparsed lines)`、append は `Imported 1 files (32 scanned, 31 skipped, 0 failed, 0 unparsed lines)`。終了 0、stderr 空。

採否: RSS の分布は重ならず、約 40–50 MiB の削減は意味がある。wall の分布は重なり、tools append の中央値増は 5.4 ms（3.0%）、tools unchanged は 0.8 ms（0.5%）。この規模の利用者の待ち時間に実質的な悪化を認めず、状態管理を追加しない局所変更として採用する。時間短縮は確定した成果としない。

代償: tools では CPU が 17.1% / 19.0% 増えた。別の非採点 run の `GODEBUG=gctrace=1` では、tools unchanged の GC 回数が 17 → 49。raw を生存させなくなると heap の余裕も減り、GC 頻度が増える観測と整合する。CPU 増の全量を GC に帰属させる profiler は取っていない。

予備計測（各版 3 回、warmup なし、10 ms 間隔の ps）では tools unchanged の中央値が .1830 → .1969 秒、candidate text の最初の run に .4645 秒の外れ値があった。監視負荷と初回起動の偏りを除くため手順を上記へ揃え直し、baseline/candidate の両方を再計測した。予備値は採否に混ぜない。

限界: 多数のファイルをまたぐ raw の保持を減らす効果であり、一個の巨大ファイルと normalized records 自体の全履歴保持は残る。親子関係を含む実ログ、より大きな入力、他 OS/機種、cold disk は性能未計測。既存の関係・履歴・full・未完成末尾・競合の正しさは core tests で確認する。保存結果の意味は変えず、既存 DB の `--full` 再取込は不要。

正しさ: baseline と candidate の `go test -count=1 ./internal/core ./internal/ingest/claudecode`、candidate の `go test -count=1 ./...` が成功。cursor 診断の unchanged・追記・prefix 変更・full を unit test で保護する。[検証ルール](../docs/verification.md) の format、vet、build、license tool install、notices check も成功した。

## 再実行

repository root で実行する。[計測 helper](claude-import-memory-measure.py) は生成時点の再現用であり、恒久的な性能 gate ではない。`ps` が sandbox で拒否される環境では計測コマンドを権限拡張して実行する。

```sh
mkdir -p .tmp/claude-performance/baseline-source
git archive 5a684691ce521d754c406ffe2637a7e41a9469ce | tar -x -C .tmp/claude-performance/baseline-source
(cd .tmp/claude-performance/baseline-source && go build -o ../baseline ./cmd/somniloq)
go build -o .tmp/claude-performance/candidate ./cmd/somniloq
python3 cache/claude-import-memory-measure.py prepare 32
python3 cache/claude-import-memory-measure.py measure baseline candidate > .tmp/claude-performance/results.jsonl
```

各結果は JSONL の `shape` / `mode` / `binary` ごとに `wall` / `cpu` / `rss` の中央値・最小・最大を取る。GC の参考観測は tools の `seed.db` を `run.db` へ複製し、各版で `GODEBUG=gctrace=1 <binary> import --config .tmp/claude-performance/tools/config.toml` の stderr の `gc ` 行を数える。最終性能値には instrumentation を使わない。
