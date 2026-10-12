---
generated_at: 2026-10-12
baseline_commit: 81e53201a20ce526e060fce3e2b2211da1d9ab22
---

# 詳細検索の行境界走査の対比較

## 対象と再現

macOS arm64、Go 1.27.1。基準版は上記 commit の変更前、候補版は `internal/core/search_matches.go` の一致後の行境界探索だけを変更した作業ツリー。同じ最適化付き build、同じ SQLite DB を使った。`search --session REF --format json -F a` のプロセス起動から出力終了までを測定した。各条件で基準版→候補版と候補版→基準版を交互に各3回、合計6組実行した。各実行は15秒で中止、出力は256 MiB超で拒否し、exit code・出力 bytes の SHA-256・`total`・`count` を比較した。すべて成功し、各条件の出力 bytes は両版で一致した。

再生成スクリプトは [search-line-generate.py](search-line-generate.py) と [search-line-measure.py](search-line-measure.py)。後者は `os.wait4` でプロセスの wall・CPU・最大 RSS を採る。入力は合成 Claude Code JSONL の6会話・各1発言。`dense_long` は2,000,000 bytes の改行なし・1,000一致、`dense_medium` は256,000 bytes・1,000一致、`dense_short` は25,600 bytes・100一致、`sparse_long` は2,000,000 bytes・1一致、`no_match` は2,000,000 bytes・0一致、`multiline` は2,002,000 bytes・2,000行・2,000一致。実行例:

```sh
mkdir -p .tmp/search-line-benchmark/baseline-source
git archive 81e53201a20ce526e060fce3e2b2211da1d9ab22 | tar -x -C .tmp/search-line-benchmark/baseline-source
(cd .tmp/search-line-benchmark/baseline-source && go build -o ../somniloq-before ./cmd/somniloq)
go build -o .tmp/search-line-benchmark/somniloq-after ./cmd/somniloq
python3 cache/search-line-generate.py
rm -f .tmp/search-line-benchmark/result.db
.tmp/search-line-benchmark/somniloq-before import --config .tmp/search-line-benchmark/config.toml
python3 cache/search-line-measure.py
```

削除対象はこの手順で生成した一時 DB のみ。比較時に import は走らない。`--limit 0` は一致の全計数と照合を含むが出力本文を持たず、`--limit 5` は5件だけ出力する。`dense_short` は明示 limit なしで100件を出力する。

## 結果

wall は6回の中央値（括弧内は範囲、単位 ms）。RSS は6回の中央値（MiB）。
各回の wall・CPU・RSS・出力 hash・件数は [生データ](2026-10-12-search-line-results.jsonl) に残した。

| 入力・出力条件 | 一致総数 / 出力数 | wall 基準→候補 | RSS 基準→候補 |
| --- | ---: | ---: | ---: |
| 2 MB・長い一行、limit 0 | 1,000 / 0 | 279.5 (278.8–282.1) → 15.8 (14.7–282.5) | 32.01 → 32.03 |
| 2 MB・長い一行、limit 5 | 1,000 / 5 | 298.5 (295.8–306.0) → 32.0 (30.4–36.4) | 86.77 → 86.81 |
| 256 KB・一行、limit 5 | 1,000 / 5 | 49.5 (49.0–50.9) → 16.8 (15.6–17.2) | 28.92 → 29.03 |
| 25.6 KB・一行、無制限 | 100 / 100 | 19.0 (18.6–20.0) → 17.5 (16.7–19.2) | 38.77 → 38.56 |
| 2 MB・一行・疎、limit 5 | 1 / 1 | 19.2 (18.5–19.7) → 19.0 (18.3–19.7) | 32.24 → 32.02 |
| 2 MB・一致なし、limit 5 | 0 / 0 | 15.5 (14.7–16.1) → 15.7 (14.3–16.8) | 32.07 → 32.07 |
| 2 MB・2,000行、limit 5 | 2,000 / 5 | 17.2 (16.0–17.3) → 17.1 (15.9–18.7) | 32.18 → 32.10 |

`dense_long`・limit 0 の候補1回は wall 282.5 ms だが CPU は21.8 ms で、他5回の wall は14.7–16.1 ms。プロセスが計時中に実行されなかった時間の影響とみる。長い一行の改善は CPU 中央値でも limit 0 が278.5→14.9 ms、limit 5 が300.4→35.9 ms。小さい・疎・複数行条件の差はばらつきと重なり、明確な変化は確認できない。

最初の候補は改行位置を通常の byte loop で集め、2 MB 疎一致で wall 19.0→19.7 ms、2 MB 複数行で16.9→18.2 msだった。最終候補では `strings.IndexByte` で改行だけを探し、上表の値へ改善した。前者の計測は同じ DB の5会話で行い、`dense_medium` を追加する前なので上表と直接の対比較ではない。

## 採否と限界

候補を採用。各 span が長い一行を前後から再走査していた処理を、発言ごとに改行位置を一度集めて再利用する処理へ変えた。長く密な一行で CLI 全体の待ち時間が大幅に減り、比較した出力とピーク RSS は実質同じ。改行数に比例した一時的な整数配列が増えるが、今回の2,000行のピーク RSS に明確な増加はなかった。

編集前の `go test -count=1 ./internal/core ./cmd/somniloq`、編集後の `goimports -w`、全 `.go` の gofmt 照合、`go test -count=1 ./...`、`go vet ./...`、`go build -o bin/somniloq ./cmd/somniloq`、`go install github.com/google/go-licenses/v2@v2.0.1`、`python3 scripts/update-third-party-notices.py --check` は成功した。`go install` は sandbox 内での初回実行時に `proxy.golang.org` の名前解決不能で終了1となり、権限を付けた再試行で終了0となった。

照合したのは合成入力と macOS arm64 の単独実行。実データの分布、さらに大きい本文、並行負荷、改行が極端に多い本文に疎な一致だけがある場合は未計測。後者では改行位置の配列確保が基準版より増え得る。`limit` は依然として全一致の計数後に適用される。正確な `total` と `--all` を保つため、ページング経路は変更していない。正しさは改行境界、ゼロ幅、複数行、重なる一致、UTF-8 byte 位置の単体テストと全体 test・vet で確認した。
