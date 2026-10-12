---
generated: 2026-10-12
baseline_commit: 36c119df72cc05a7ba2147ecb6da5412d78fe9ab
candidate: この記録と同じ差分の internal/core/import_claude.go
---

# Claude 会話比較の反復走査

採用: 通常 import で変更対象の会話があるときだけ、snapshot を一度走査して対象会話の record 参照と全ファイルの title・name 証拠を集める。従来は変更会話ごとに全 record を走査していた。本文と metadata の比較、保存済み本文の保持、`imported_at` の判定は維持する。索引は一回の import 中だけ保持し、full import や変更会話がない場合は作らない。

## 条件

- macOS Darwin 27.0.0 arm64、M4 MacBook Pro、Go 1.27.1。通常の `go build`、Go runtime は既定。合成した Claude JSONL で、既存 SQLite seed から一部の会話へ assistant record を一件ずつ追記する。実ログは使わない。
- 128 ファイル × 128 record、96 会話変更と、512 ファイル × 32 record、384 会話変更を比較した。いずれも初期 16,384 record、入力約 6.3 MB。初回取込と DB 複製は計時外。各実行前に同じ seed DB に戻し、両版を交互に直列実行した。stdout の取込件数、終了 0、stderr 空を確認した。
- 操作全体の wall は monotonic clock、CPU とピーク RSS は `wait4` で測る。別 build に `runtime.MemStats` と時刻を比較区間の前後だけに入れ、比較時間と累積 allocation を調べた。この instrumented build の wall/CPU は採否の操作全体値に混ぜない。
- 30 秒／process、512 MiB RSS、1 GiB 一時ディスクを予算とする。`ps` が許可される環境では 100 ms ごとに RSS を監視する。今回の sandbox では `ps` が拒否されたため、終了後の `wait4` peak の確認に限られた。全 run は予算内で終了した。

## 結果

中央値（最小–最大）。wall・CPU・比較時間は秒。RSS は MiB。比較区間の allocation は MiB。

| 規模・指標 | baseline | candidate |
|---|---:|---:|
| 128 会話・比較時間、各 4 回 | .0541 (.0520–.0555) | .0394 (.0385–.0401) |
| 128 会話・比較 allocation、各 4 回 | 29.16 | 29.37 |
| 128 会話・全体 wall、各 4 回 | .8481 (.8415–.8554) | .8328 (.8294–.8353) |
| 128 会話・全体 CPU、各 4 回 | .8912 (.8886–.8980) | .8778 (.8767–.8818) |
| 128 会話・peak RSS、各 4 回 | 47.53 | 47.48 |
| 512 会話・比較時間、各 4 回 | .1179 (.1147–.1371) | .0555 (.0551–.0653) |
| 512 会話・比較 allocation、各 4 回 | 30.04 | 30.46 |

512 会話の操作全体は合計 21 対で baseline wall 中央値 .9872 秒（.9680–1.0378）、candidate .9377 秒（.9045–1.1790）。CPU は 1.0495 秒（1.0367–1.0869）→1.0009 秒（.9773–1.1202）。最初の 4 対では wall 中央値 1.0042→1.0741 秒となり candidate が遅かった。後続の 6 対では .9838→.9377 秒、別の 6 対では .9830→.9363 秒、最後の 5 対では .9893→.9230 秒だった。最初の組を含めると操作全体の効果には環境由来のばらつきがあり、約 5% は安定した保証値ではない。比較区間の短縮は両規模で分布が重ならず、反復走査の削除に対応する。

ピーク RSS は 512 会話の全 21 対で 47.98 MiB（47.36–48.52）→48.05 MiB（47.30–48.69）。比較区間の allocation は 128 会話で約 0.21 MiB、512 会話で約 0.42 MiB 増加した。対象 record のポインタと metadata 証拠を import 中に保持する代償であり、peak RSS の有意な減少は見られない。多くの会話が追記される場合の比較 CPU を減らす価値が、局所的な索引構築と小さい allocation 増を上回ると判断した。

追加の入口確認では、128 会話の unchanged、1 会話だけ変更、新会話 1 件の各 4 対で全体 wall の分布が重なり、明確な改善・悪化は見られなかった。新会話では DB に既存会話がなく比較を早期終了するため、索引構築は余分な仕事になる。影響は今回の規模で観測限界内だった。

正しさ: 変更前に `go test -count=1 ./internal/core ./internal/ingest/claudecode` が成功した。変更後には同じテストと、record のあるファイルと metadata だけの別ファイルを使った回帰テストで、title・name 更新、本文と履歴の保持を確認した。既存テストは空白追記時の `imported_at` 保持、本文変更時の更新、複数ファイルの履歴・親子関係、full import と失敗時の旧状態保持を確認する。変更後の共通 gate は、Go format check（終了 0、0.58 秒）、`go test -count=1 ./...`（終了 0、6.13 秒）、`go vet ./...`（終了 0、0.10 秒）、`go build -o bin/somniloq ./cmd/somniloq`（終了 0、0.27 秒）、`go install github.com/google/go-licenses/v2@v2.0.1`（権限拡張下で終了 0、0.64 秒）、`python3 scripts/update-third-party-notices.py --check`（終了 0、2.94 秒）。`goimports -w` を変更した Go ファイルに実行した。通常 sandbox の初回 `go install` は DNS 拒否で終了 1 となった後、権限拡張で成功した。

限界: 合成した小さめの record とローカル SQLite の結果であり、大きな payload、実ログの親子関係、cold disk、他機種・OS は性能未計測。関係解決や保存の負担が支配的な入力では操作全体の効果が小さくなる。保存形式は変えないため既存 DB の `--full` 再取込は不要。

## 再実行

[計測 helper](claude-owner-comparison-measure.py) は生成時点の再現用であり、恒久的な性能 gate ではない。repository root で次を実行する。`instrument` はコピーした source を書き換えるので、再実行時は source を作り直す。

```sh
mkdir -p .tmp/claude-owner-comparison/baseline-source .tmp/claude-owner-comparison/candidate-source
git archive 36c119df72cc05a7ba2147ecb6da5412d78fe9ab | tar -x -C .tmp/claude-owner-comparison/baseline-source
git archive 36c119df72cc05a7ba2147ecb6da5412d78fe9ab | tar -x -C .tmp/claude-owner-comparison/candidate-source
cp internal/core/import_claude.go .tmp/claude-owner-comparison/candidate-source/internal/core/import_claude.go
(cd .tmp/claude-owner-comparison/baseline-source && go build -o ../baseline ./cmd/somniloq)
(cd .tmp/claude-owner-comparison/candidate-source && go build -o ../candidate ./cmd/somniloq)
python3 cache/claude-owner-comparison-measure.py prepare 512 32 384
python3 cache/claude-owner-comparison-measure.py instrument
python3 cache/claude-owner-comparison-measure.py measure 512 384 > .tmp/claude-owner-comparison/results.jsonl
```

128 会話の入力は `prepare 128 128 96` と `measure 128 96` に替える。`mode=plain` の `wall` / `cpu` / `peak_rss`、`mode=instrumented` の `compare_seconds` / `compare_alloc_bytes` / `compare_mallocs` を版ごとに集計する。helper の `prepare` は自身の入力と seed DB を再作成する。
