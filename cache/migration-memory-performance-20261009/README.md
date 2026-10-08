---
observed_at: 2026-10-09
compiled_from_commit: 0ba4b0a18e11c51af7e9731c37f8b7367104ae30
---

# 移行時の本文メモリ保持

`Migrate` の全入力を先に解析する段階で、本文 bytes と正規化済み発言を全 group 分保持していた。候補版はこの段階に所属・失敗・物理行・snapshot hash の index を作り、全入力の競合と旧行候補を確定してから、各 group の本文を読み直して置換する。最大会話を約43 KiBに固定した総量10.652 MiBでは、初回 CLI のピーク RSS 中央値が98.02→48.02 MiB（50.00 MiB減）になった。本文64 MiBでは300.61→64.08 MiB（236.53 MiB減）。単一の64 MiB会話でも439.70→372.91 MiB（66.79 MiB減）だった。この変更を採用した。

## 比較条件と再実行

基準版は上記commitのclean source、候補版はこの記録時点の作業ツリー。変更前の `go test -count=1 ./...` はexit 0、所要約2.8秒だった。基準sourceは編集前に `.tmp/memory-performance/before-source/` に保存し、両版を同じ時期・同じ入力・未存在の移行先DBから交互に実行した。macOS 27.0.1 / 26A434、Go 1.27.1 darwin/arm64、通常 `go build`、`GOFLAGS=-buildvcs=false`。入力生成、build、DBの検証は計時外。「初回」は移行先DB未存在を意味し、filesystem cacheは冷却していない。

[measure.py](measure.py) は既存の[合成入力と保存検証](../migration-performance-20261008/measure.py)を再利用する。各条件で基準/候補の順を交互にして3組、初回→同一snapshot再実行を行い、最後に累積allocationとGit起動数用の計測器付き1組を実行した。5条件、計80操作。`/usr/bin/time -l` の時間・RSSは計測器なし3回の中央値と範囲。`runtime.MemStats.TotalAlloc` は累積割当量であり、ピーク保持量ではない。Git起動数・allocationは計測器付きの初回1回。CLIの子process RSSは250 msごとに監視し、各操作60秒・CLI RSS 1 GiB・専用領域2 GiBを上限とした。上限中断はなかった。最初のsandbox内試行は `pgrep` がprocess一覧を取得できず中止し、その入力を作り直してsandbox外で全比較した。修正途中の候補値も採否から除いた。

再実行時は自分の計測processが終了したことを確認し、この専用一時領域だけを除去する。製品source、実ログ、実DBには触れない。macOSの `time -l`、`pgrep`、`ps` が使える環境でリポジトリrootから実行する。基準sourceを次のように復元し、候補は再実行時の作業ツリーを使う。

```sh
mkdir -p .tmp/memory-performance/before-source
git archive 0ba4b0a18e11c51af7e9731c37f8b7367104ae30 go.mod go.sum internal cmd | tar -x -C .tmp/memory-performance/before-source
python3 cache/migration-memory-performance-20261009/measure.py
```

生値、入力のbytes・最大会話・件数、各操作の保存確認結果は [results.json](results.json)。再実行出力は `.tmp/memory-performance/results.json`。全条件の正常完了group数・初回コピー有無・旧行削除/他source保持数・会話/発言数・本文長と合計・role/membership/timestamp・cursor終端・SQLite integrity・移行元digestとsidecar不在を毎操作で確認した。

## 操作全体の結果

| 条件（総入力 / 最大会話） | 基準 初回RSS MiB | 候補 初回RSS MiB | 基準 初回 秒 | 候補 初回 秒 | 累積allocation MiB 基準→候補 | Git起動 基準→候補 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 0.664 / 0.042 MiB | 33.06 [32.86–33.55] | 31.19 [31.16–31.56] | 0.13 [0.12–0.21] | 0.13 [0.13–0.21] | 21.80→28.05 | 2→2 |
| 2.660 / 0.042 MiB | 46.14 [45.72–46.28] | 35.28 [35.22–35.77] | 0.45 [0.45–0.45] | 0.50 [0.47–0.50] | 76.22→101.15 | 2→2 |
| 10.652 / 0.042 MiB | 98.02 [97.61–98.64] | 48.02 [47.84–48.50] | 2.80 [2.79–2.95] | 2.92 [2.92–2.97] | 318.90→418.33 | 2→2 |
| 本文64 MiB（64.165 / 4.010 MiB） | 300.61 [299.44–306.81] | 64.08 [63.66–64.12] | 0.69 [0.67–0.72] | 0.95 [0.95–0.96] | 943.09→1427.81 | 2→2 |
| 単一会話64 MiB（64.163 / 64.163 MiB） | 439.70 [439.70–439.86] | 372.91 [354.52–375.77] | 0.67 [0.66–0.67] | 0.92 [0.92–0.93] | 941.99→1423.61 | 2→2 |

同一snapshot再実行のRSS中央値も、総量10.652 MiBで93.00→43.08 MiB、本文64 MiBで297.62→59.52 MiB、単一会話64 MiBで435.39→362.77 MiB。再実行時間はそれぞれ3.18→3.31秒、0.88→1.22秒、0.87→1.13秒。全範囲は生値にある。本文をindex作成時と置換時に2回解析するため、allocationと時間は増える。総量10.652 MiBの初回時間差は測定範囲が一部重なるため小さいが、本文64 MiBの増加は範囲を超える。RSS削減が主目的であり、この時間・allocation代償を受け入れた。

## 保存契約と残る制約

index passは、失敗group、継承文脈、重複排除前の物理行も所属証拠として保持する。全入力の証拠から旧行候補を決めるまでは旧行を削除しない。`migrationInput.Files` は一回の `Migrate` の間保持し、各会話の置換前とcommit前に全ファイル集合を走査する。本文の読み直しではindexの各path/hashを照合し、commit前にも元のsnapshot hashを確認する。読み直し間の変更が元に戻る場合でも異なる本文を確定しない。repository解決は一回の `Migrate` 全体で再利用し、通常importのbuilder寿命は変えない。旧行UUID照合SQLは変更していない。

既存migration fixtureの初回・同一snapshot再実行・拒否・行単位置換・失敗時保持・重複物理行・snapshot変更と、通常importの差分skip/診断/fullを全package testで確認した。追加テストはindexがowner/hash/全物理行を残し本文を保持しないこと、本文読み直しのhash不一致を拒否すること、migration内のrepository解決再利用を確認する。CLI性能入力は正常系の合成ログで、失敗groupや並行更新の性能は計測していない。単一会話のRSSはなお会話本文に依存し、GiB級総入力・実ログ・多数の失敗診断・複数入力root・cold cacheへ上限を外挿しない。

## gate

`goimports -w` 後に `docs/verification.md` の全共通gateを実行した。記録できた終了コードと所要時間は次のとおり。format・test・vet・build・installの最終実行は並列バッチ全体で約3.1秒。バッチ内の個別所要時間は記録していない。

| command | exit code | 所要時間 |
| --- | ---: | ---: |
| `goimports -w internal/core/migrate.go internal/core/migrate_memory_test.go internal/ingest/codex/adapter.go internal/ingest/codex/migration_test.go` 等の変更Goファイル整形 | 0 | 未記録 |
| `find . -name '*.go' -type f -print0 \| xargs -0 gofmt -l` と空結果の確認 | 0 | 個別値は未記録 |
| `go test -count=1 ./...`（変更後の独立実行） | 0 | 約3.3秒 |
| `go test -count=1 ./...`（最終gate） | 0 | 個別値は未記録 |
| `go vet ./...` | 0 | 個別値は未記録 |
| `go build -o bin/somniloq ./cmd/somniloq` | 0 | 個別値は未記録 |
| `go install github.com/google/go-licenses/v2@v2.0.1`（一時 `GOBIN`、sandbox外の再実行） | 0 | 約6.7秒 |
| `python3 scripts/update-third-party-notices.py --check` | 0 | 約6.7秒 |
| `git diff --check`、Python構文・生値整合確認 | 0 | 個別値は未記録 |

installはsandboxのnetwork制限で最初の試行がexit 1（所要時間未記録）になり、上表の再実行で成功した。build時のGo cache stat書き込み警告はexit 0で、専用 `GOCACHE` を使用した。CLI公開形式とschemaは変えていない。
