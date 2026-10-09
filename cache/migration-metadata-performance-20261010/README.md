---
generated_at: 2026-10-10
baseline_commit: 36834052e81f340c387e8413a4b795478a2f1d60
candidate: 同 commit に対する本変更の working tree
---

# migrate metadata 所属確認の対比較

`results.json` は専用合成ログ・新規旧形式 DB の結果。実 DB・実ログは使用していない。再現は repository root で次を実行する。初期状態へ戻すため各対で専用の移行先 DB だけを削除し、初回 migrate と同一 snapshot 再実行を順に測る。baseline は `git archive`、candidate は working tree のコピーから同じ Go build 条件で作る。計測用コードは `.tmp/` のコピーにだけ挿入する。

```bash
SOMNILOQ_PERF_BASELINE=36834052e81f340c387e8413a4b795478a2f1d60 \
SOMNILOQ_PERF_WORK="$PWD/.tmp/migration-metadata-new-run" \
python3 cache/migration-metadata-performance-20261010/measure.py
```

macOS `/usr/bin/time -l` の RSS 取得には sandbox 外の sysctl が必要だった。失敗した sandbox 計測は採否に使っていない。Go/OS は JSON に記録、`GOFLAGS=-buildvcs=false`、`GOCACHE=.tmp/go-cache`、その他 GC 設定は既定。各 run は60秒 timeout、完了時 RSS 1GiB、ケース間の作業容量2GiB、計測全体15分を上限とした。全操作は直列。入力は一度生成し、各対で同じログ・snapshot・設定を使用する。ファイル cache を強制排除していない。base 初回1対目は両版とも後続対より遅く、cold/warm の混在として値を残す。

## 採否

各3対の通常 build を A/B、B/A、A/B の順に実行。中央値と最小〜最大は秒。instrumentation 付きの追加1対はこの集計に含めない。

| 合成入力 | 初回 baseline → candidate | 同 snapshot 再実行 baseline → candidate |
| --- | --- | --- |
| 64会話/64ファイル、4,096本文、2,783,926 bytes | 0.41 (0.40–0.73) → 0.41 (0.38–0.71) | 0.45 (0.43–0.46) → 0.44 (0.42–0.44) |
| 256会話/256ファイル、16,384本文、11,147,794 bytes | 2.33 (2.32–2.37) → 2.27 (2.23–2.29) | 3.10 (3.08–3.11) → 3.06 (3.04–3.09) |
| 16会話/16ファイル、1,024本文、67,280,070 bytes | 0.84 (0.80–0.92) → 0.61 (0.60–0.63) | 1.04 (1.00–1.06) → 0.82 (0.79–0.86) |

64MiB本文ケースで初回27.4%、再実行21.2%短縮を採用根拠とした。256会話の小本文では初回2.6%の小差、64会話では時間改善を確認できない。大きな本文で同じ置換結果のまま重複した解析・ハッシュ読み取りが減る一方、小本文の多数会話では保存処理が支配する。

64MiBケースのピーク RSS（通常 build、中央値と範囲、MiB）は初回56.39 (56.05–57.03) → 57.00 (56.89–57.20)、再実行51.52 (51.16–52.06) → 52.44 (52.30–52.64)。メモリ改善は主張しない。1MiB未満の増加を伴うが、全件本文67MiBの保持は導入していない。64→256会話の初回RSS中央値は baseline31.25→37.69、candidate31.34→37.47MiB。一 group の最大入力は小本文43,574 bytes以下、64MiBケース4,205,045 bytes以下。計測値だけで任意の入力サイズの上限を保証しない。

## 原因の内訳

追加1対の instrumentation は、初回所属確認、per-group全文解析、commit前の確認、SQL/保存（commit前確認を含む）を直接計時した。64MiBケースの初回値:

| 工程 | baseline | candidate |
| --- | ---: | ---: |
| 初回 index | 176.78ms | 0.47ms |
| per-group 全文解析 | 189.19ms | 201.01ms |
| 内容hash照合 / stat確認（合計） | 55.43ms | 0.36ms |
| SQL/保存（commit前確認を含む） | 361.86ms | 338.77ms |
| 累積 allocation | 1,058,499,792 bytes | 627,608,560 bytes |

工程の列は一部重複するので合計しない。単一 probe 値は時間差の採否には使用しない。`full_reads` と `hash_reads` はファイル全体を読む処理回数、candidate の `*_calls` は Reader.Read 呼び出し回数（EOF含む）であり、同じ単位ではない。

baseline は全文読み32回/134,560,140 bytesと追加hash読み32回/134,560,140 bytes。candidate は全文読み16回/67,280,070 bytesと metadata 読み16回/65,536 bytes。metadata の bytes は bufio の先読み込みを含む Reader 実測、全文 bytes と hash bytes も読取実測。合計269,120,280 → 67,345,606 bytes（75.0%削減）。通常 import が変化検出に使う解析 bytes のSHA-256は双方とも保存しており、candidate の追加hash照合読みだけをなくした。kernelの物理disk I/O量は測っていない。

通常 import は初回全件保存と差分skipをそれぞれ別に実行。64MiBケースの単発参考値は baseline初回0.637s/skip0.215s、candidate初回0.683s/skip0.202s。migrate全件置換と差分skipを同じ処理量として比較しない。通常import自体の改善は主張しない。

## 正しさ・試行・限界

各実行で終了0、置換件数、本文件数、legacy保持・削除件数、cursor EOF、SQLite integrity、移行元snapshot bytesとsidecar不変を確認。各対の全 table の論理行（imported_atを除く）は一致した。追記の収束、文脈・親子・cursor、削除・移動・置換・縮小・同サイズ変更の失敗時保持は `internal/core/migrate_append_test.go`、既存の移行 fixture は0件置換・別ID保持・所属競合・不完全入力・保存/commit失敗・再実行を確認する。共通gateの command/exit/duration は `gates.json`、build済CLIの成功・同snapshot再実行・不完全入力の失敗時保持・helpとstdout/stderr/exitは `cli-checks.json`。既存の無関係な `.tmp/log-memory-estimate/main.go` の未整形でformat gateが一度失敗し、同一作業の取りまとめ側がその一時物を整形して再実行は成功。go-licenses installもsandbox内DNS失敗後、固定版の取得をsandbox外で再実行して成功した。

最初の試行は opened file の `io.ReadAll` で初回RSS中央値56.3→59.8MiBに増えたため採用せず、サイズ事前確保へ変更した。結果は `preliminary-readall-results.json` に保存。最終比較では通常 import の `os.ReadFile` 経路も維持した。

正常な追記を受理する代わりに内容不変の全hash再照合は行わない。保存前の strict parse と stat で検出できる破損・削除・移動等の保持を保証する。prefix書換えと伸長を組み合わせた変更、mtimeを復元した編集、stat後〜commitまでの同時編集は未保証で、移行中の既存prefix編集・削除・移動を禁止する利用契約を `docs/migration.md` に置く。通常 import を破損・欠落の修復に使う保証は設けない。

実環境17,231会話/約15.95GB、巨大単一会話、低速storage、長期の並行書き込みは未計測。過去の31分20秒の実行内訳を今回の合成結果から推定しない。
