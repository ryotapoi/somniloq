# Codex 通常 import の会話単位読み込み（2026-10-10）

- 基準: `cd4ac1316849fb44d640bc8abe6a9dbb6c7202c9`。候補: この記録と同じ作業ツリーの変更。Go 1.27.1、macOS 27.0.1 arm64。`GOGC`、`GOMEMLIMIT`、`GOMAXPROCS`、`GOFLAGS` は未設定。
- 再生成: リポジトリ直下で `GOCACHE=$PWD/.tmp/go-cache python3 cache/codex-import-performance-20261010/measure.py`。入力、DB、計測用ソース・実行ファイルは `.tmp/codex-import-performance-20261010/` に作る。既存ディレクトリは手動で取り除いてから再実行する。
- 合成 JSONL と専用 DB のみ。各 run の初回 import は空 DB、`--full --yes` は同じ合成入力で初回取り込み済みの DB を再構築する。各 run は60秒、ピーク RSS 1 GiB、作業容量 2 GiB、全計測15分を上限とした。実際は全 run が上限内。測定は直列、各ケースの A/B 順序を反転して3回実行した。
- `measure.py` が基準・候補のソースコピーだけに読取 byte counter と `getrusage(RUSAGE_SELF)` を挿入して測る。計時は CLI プロセス全体の壁時計時間。DB の `inputs`、`sessions`、`messages`、`import_state` の全列（実行時刻だけ除外）の digest と SQLite integrity check を照合した。全 run で取込失敗0、論理行 digest 一致。

| 入力 | 操作 | 基準時間中央値（範囲） | 候補時間中央値（範囲） | RSS 基準→候補 | 読取量 基準→候補 |
| --- | --- | ---: | ---: | ---: | ---: |
| 16会話、最大0.50 MiB、計8.0 MiB | 初回 | 0.029秒（0.028–0.192） | 0.036秒（0.031–0.146） | 32.4→22.8 MiB | 8.0→16.0 MiB |
| 同上 | `--full` | 0.023秒（0.022–0.023） | 0.026秒（0.024–0.050） | 32.1→22.6 MiB | 8.0→16.0 MiB |
| 64会話、最大0.50 MiB、計32.0 MiB（イベント主体） | 初回 | 0.088秒（0.085–0.089） | 0.099秒（0.095–0.103） | 69.8→25.2 MiB | 32.0→64.0 MiB |
| 同上 | `--full` | 0.062秒（0.059–0.063） | 0.072秒（0.070–0.073） | 71.2→25.2 MiB | 32.0→64.0 MiB |
| 64会話、最大0.50 MiB、計32.1 MiB（本文主体） | 初回 | 0.296秒（0.287–0.309） | 0.318秒（0.308–0.360） | 152.6→31.8 MiB | 32.1→64.2 MiB |
| 同上 | `--full` | 0.395秒（0.385–0.406） | 0.424秒（0.413–0.567） | 151.6→32.1 MiB | 32.1→64.2 MiB |
| 1会話、最大32.0 MiB | 初回 | 0.061秒（0.060–0.065） | 0.064秒（0.062–0.064） | 82.1→80.9 MiB | 32.0→64.0 MiB |
| 同上 | `--full` | 0.056秒（0.056–0.061） | 0.063秒（0.062–0.064） | 82.3→80.9 MiB | 32.0→64.0 MiB |

採用。会話数が増えても全会話の本文・元 bytes が同時に常駐せず、32 MiB の複数会話入力では RSS が約62–79%減った。本文主体の操作時間は中央値で初回約7%、`--full` 約7%増え、読み取りは全ケースで2倍。これは保存前に全ファイルの読み取り可能性と所属を確認し、その後会話ごとに本文を読む費用。巨大な単一会話では最大会話の本文を一度に構築するため RSS はほぼ変わらない。小規模の冷えた初回 run はばらつきが大きく、時間差の精密な推定には使わない。

関連 package の変更前テストは `go test -count=1 ./internal/core ./internal/ingest/codex` で成功。変更後は同じテストと全体テスト・vet・build・整形・第三者通知 check が成功した。`go install github.com/google/go-licenses/v2@v2.0.1` は sandbox の DNS 制約で初回失敗後、ネットワーク許可付きで成功した。ビルドした CLI の合成1ファイル入力で初回・`--full --yes` の成功、非対話 `--full` の拒否を確認した。継承・順序・重複・差分 cursor・同時 import・full 原子性は既存テストに加え、preflight／後段読取失敗の保存保持、会話間の repository 解決再利用テストで確認した。実利用 DB と測っていない会話サイズ分布への外挿はしない。

個別値と失敗件数・論理行 digest は [results.json](results.json)、gate の command・exit code・duration は [gates.json](gates.json)、CLI 実行結果は [cli-smoke.json](cli-smoke.json) に保存した。
