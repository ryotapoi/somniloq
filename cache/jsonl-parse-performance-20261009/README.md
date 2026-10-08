---
observed_at: 2026-10-09
compiled_from_commit: 488636fe11a51bd4a30cab89b25136b13e6e3fcf
---

# 移行時の JSONL 重複解析

strict 移行の各物理行では、所属検証と保存用処理がそれぞれ `ParseRecord` を実行していた。移行 index と会話単位の本文読み直しの両方で起きる。候補は一つの物理行に限って解析結果を共有する。所属検証は引き続きメタデータ適用前に行い、最初の metadata は保存 handler が検証する。後続 metadata の owner・parent・境界競合は strict 検証で先に判定する。通常 import の部分取り込み経路は維持する。

64 MiB 本文入力の初回移行で、操作全体の中央値は **0.94→0.83 秒**、user+sys CPU は **1.05→0.95 秒**、累積 allocation は **1427.76→1283.67 MiB** になった。大きな非本文 record 入力でも初回は **0.21→0.18 秒**、CPU **0.25→0.21 秒**、allocation **202.24→164.67 MiB**。総量16倍・短い行の入力では待ち時間の差はばらつきから区別できなかった。大きい行で操作全体への効果があり、変更は行内の解析結果共有に限定できるため採用した。ピーク RSS の低下は採用根拠にしていない。

## 比較条件と再実行

基準は上記 commit の clean source、候補はこの記録時点の作業ツリー。変更前の `go test -count=1 ./...` は exit 0、約3.7秒。macOS 27.0.1 / 26A434、Apple M4 Pro、Go 1.27.1 darwin/arm64、通常 `go build`、`GOFLAGS=-buildvcs=false`。入力生成、build、DB検査は計時外。移行先 DB 未存在からの初回と、同一 snapshot の再実行を各組で連続実行した。filesystem cache は冷却していない。

作業領域は gitignore 済みの `.tmp/jsonl-performance/`。製品 source・実ログ・実 DB は計測に使わない。1操作60秒、CLI RSS 1 GiB、専用領域2 GiBを上限とし、250 msごとに子 process を監視した。上限中断はない。最初の sandbox 内試行は macOS の `pgrep` が process 情報を取得できず中止したため、監視可能な環境で全条件を測り直した。最終候補の小さな整理後に A/B 全条件を再実行し、下表と生値はその最終試行だけを採用した。

再実行前に自分の計測 process が終了したことを確認し、この専用領域だけを除去する。リポジトリ root から macOS の `/usr/bin/time -l`、`pgrep`、`ps` が使える環境で実行する。

```sh
mkdir -p .tmp/jsonl-performance/before-source
git archive 488636fe11a51bd4a30cab89b25136b13e6e3fcf go.mod go.sum internal cmd | tar -x -C .tmp/jsonl-performance/before-source
python3 cache/jsonl-parse-performance-20261009/measure.py
```

[measure.py](measure.py) は既存の[合成入力・保存検証](../migration-performance-20261008/measure.py)を再利用する。4条件を基準・候補の順を交互にして3組、初回→同一snapshot再実行の順に計測し、追加の計測器付き1組で累積 allocation を取った。合計64操作すべて exit 0。時間・CPU・RSS は計測器なし3回の中央値と範囲。allocation は計測器付き1回の `runtime.MemStats.TotalAlloc` であり、ピーク保持量ではない。[results.json](results.json) に全生値、入力寸法、移行 summary を保存した。再実行出力は `.tmp/jsonl-performance/results.json`。

| 入力（総 MiB / 最大会話 MiB） | 初回 秒 基準→候補 | 初回 CPU 秒 基準→候補 | 初回 allocation MiB 基準→候補 | 初回 RSS MiB 基準→候補 |
| --- | --- | --- | --- | --- |
| 基準（0.664 / 0.042） | 0.13 [0.12–0.22] → 0.13 [0.12–0.55] | 0.13 [0.12–0.13] → 0.13 [0.13–0.13] | 28.02→26.64 | 31.36→31.14 |
| 総量16倍（10.652 / 0.042） | 2.77 [2.77–2.83] → 2.75 [2.69–2.77] | 2.91 [2.88–2.95] → 2.87 [2.81–2.87] | 418.42→396.07 | 49.12→48.55 |
| 非本文大 record（16.730 / 1.046） | 0.21 [0.20–0.21] → 0.18 [0.17–0.18] | 0.25 [0.24–0.26] → 0.21 [0.21–0.21] | 202.24→164.67 | 37.05→35.89 |
| 本文64 MiB（64.165 / 4.010） | 0.94 [0.90–0.94] → 0.83 [0.81–0.87] | 1.05 [1.03–1.07] → 0.95 [0.90–0.98] | 1427.76→1283.67 | 65.25→62.50 |

同一 snapshot 再実行の時間中央値は、基準0.12→0.12秒、総量16倍3.12→3.11秒、非本文大 record 0.20→0.18秒、本文64 MiB 1.10→1.02秒。CPU は同順で0.13→0.13、3.24→3.22、0.24→0.22、1.19→1.10秒。全範囲と allocation・RSS は生値にある。小さい基準入力の初回は一回の候補値0.55秒を含み、速度改善を主張しない。総量16倍の初回・再実行も範囲が重なる。strict 検証と保存 handler を個別には計時せず、両者で重複していた一回の `ParseRecord` だけを除く A/B 差を、その削減可能費用として直接測った。大行条件の CPU・allocation と操作全体の減少が採用判断に足りたため、計測器の負荷が入る区間別の寄与率は求めていない。

## 保存契約と検証

比較 script は毎操作で正常完了 group 数、初回コピー有無、旧行削除と他 source 保持、会話/発言数、本文長、role・membership・timestamp、cursor終端、SQLite integrity、移行元 digest と sidecar 不在を確認した。既存の migration fixture は、初回・同一 snapshot 再実行・拒否・行単位置換・失敗時保持、埋め込み親と本人境界、payload ID 重複、物理行、診断、通常 import の部分取り込みを全 package test で確認する。候補は保存形式と schema を変えないため `--full` 案内は不要。

最終ソースの `goimports -w`、format 空確認（exit 0、約0.32秒）、`go test -count=1 ./...`（exit 0、約3.41秒）、`go vet ./...`（exit 0、約0.07秒）、`go build -o bin/somniloq ./cmd/somniloq`（exit 0、約0.16秒）、`go install github.com/google/go-licenses/v2@v2.0.1`（exit 0、約0.55秒）、`python3 scripts/update-third-party-notices.py --check`（exit 0、約2.48秒）を確認した。`go install` は sandbox の network 制限で初回 exit 1、監視可能な環境で再実行して成功した。notice check は Go cache の stat 書き込み警告を出したが exit 0。

性能入力は正常系の合成ログであり、多数の失敗診断、複数入力 root の競合、並行更新、実ログ、cold cache、GiB 級入力は測っていない。非本文 record は各発言の後の16 KiB `event_msg`。本文64 MiBは16会話・各64発言で、最大会話約4 MiB。小行を多数含む普通の入力に同じ改善率を外挿しない。
