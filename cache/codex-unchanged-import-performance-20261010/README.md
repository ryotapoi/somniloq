# Codex 変更なし会話の差分 import 計測（2026-10-10）

基準版は `c4af9909e4499860c8289db76d1909042a860373`、変更版はこの変更の作業ツリー。macOS 27.0.1 / Apple M4 Pro / Go 1.27.1 で、専用の合成 JSONL と一時 SQLite DB だけを使った。32 会話、各 80 発言、計 1,735,574 bytes、最大会話 54,287 bytes。各 CLI ケースは同じ初期状態から 3 対を交互の A/B 順で実行し、終了コード・発言件数・SQLite integrity・保存内容 digest を照合した。数値は操作全体の壁時計時間の中央値（最小–最大）。生値は [results.json](results.json)、再実行手順は [measure.py](measure.py)。

| ケース | 基準版 ms | 変更版 ms | 中央値の差 |
| --- | ---: | ---: | ---: |
| 全件変更なし | 25.0 (21.4–46.1) | 12.2 (12.2–13.9) | -12.8 ms / 2.05 倍 |
| 1 会話へ追記 | 35.4 (32.9–44.5) | 22.2 (21.6–22.3) | -13.2 ms / 1.60 倍 |
| 1 会話の同サイズ編集 | 30.5 (29.3–44.4) | 23.4 (20.1–23.8) | -7.1 ms / 1.31 倍 |
| 1 会話へ rollout 追加 | 30.6 (29.2–39.3) | 22.4 (22.1–36.0) | -8.2 ms / 1.36 倍 |
| 初回 import | 261.6 (242.7–279.9) | 254.8 (229.8–298.5) | ばらつき内 |
| `--full` | 209.7 (199.2–224.9) | 206.6 (197.5–209.6) | ばらつき内 |

同形状の変更なし import を package benchmark でも 3 対、各 5 回実行した。allocation は基準版の中央値 17,458,488 B/op・42,332 allocs/op から、変更版 345,604 B/op・2,594 allocs/op へ減少した。生値は [allocation-results.json](allocation-results.json)、一時的にソースへ入れる計測コードと手順は [alloc_bench.go.txt](alloc_bench.go.txt)・[measure_alloc.py](measure_alloc.py)。benchmark の時間は CLI 全体の待ち時間ではない。

`BuildImportIndex` は両版とも各ファイルを最後まで読む。変更版は保存済み cursor が全 path にある会話だけ追加の streaming SHA-256 読み取りを行い、全 hash 一致時に本文構築と JSONL 解析を省く。変更会話は hash で不一致を見つけるまでの追加読み取り後、全 rollout を再読込・再構築する。初回は cursor がなく、`--full` は hash preflight を通らない。したがって改善は主に解析・allocation の削減であり、読み取り bytes の削減ではない。保存済み path が走査から消えた場合は所属を特定できないため、この input の全会話で早期スキップを止める。通常 import の既存の削除扱いは変更していないので、stale path が残る間はこの性能上の制限が続く。

baseline の対象 package テストを変更前に実行し成功。変更後は `go test -count=1 ./...`（約 3.3 秒）、`go vet ./...`（約 1.4 秒）、`go build -o bin/somniloq ./cmd/somniloq`（約 0.2 秒）、gofmt 確認（約 1.4 秒）、`go install github.com/google/go-licenses/v2@v2.0.1`（約 0.6 秒）、`python3 scripts/update-third-party-notices.py --check`（約 2 秒）が成功した。`go install` は sandbox 内で proxy の名前解決に失敗したため、昇格実行で成功を確認した。`HashFile` 後に追記してから `BuildGroups` が解析した bytes に対応する hash/cursor を作ること、hash の読み取り失敗、走査から消えた保存済み path では早期スキップを止めることも個別テストで確認した。既存の import テストは未完了末尾・読み取り失敗・cursor 競合・保存失敗を確認する。この 1.7 MB 入力より大きい会話数・単一会話・別環境は今回測っていない。
