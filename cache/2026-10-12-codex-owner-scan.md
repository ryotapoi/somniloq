---
generated: 2026-10-12
baseline_commit: 9df3e19b5edba0bae82c4b20baeb504ea0431cbd
candidate: この記録と同じ差分の internal/core/db_write.go と internal/core/import_codex.go
---

# Codex owner 置換の反復走査

採用: owner の置換 DELETE（本文・会話）と通常 import の欠落 rollout 保持 SELECT（本文・metadata）を、既存保存と同じ `rootIdentity(sessionID)` の identity 条件へ揃えた。schema・索引は変更しない。対象会話の更新、body / context / unresolved、保存済み欠落本文・metadata、cursor と transaction の保証を維持する。

## 条件と採否

Darwin 27.0.0 arm64、Go 1.27.1、project の modernc SQLite 3.45.1、通常 `go build`。初期 git 差分なし。実ログを使わず、owner ごとの session metadata と 256 byte の user 本文を合成した。owner 数・owner あたり本文数を変え、通常初回 import と migrate 初回を空の移行先から測定した。migrate 元は fixture の既知 schema の空 standalone DB で、旧履歴コピー・削除量を増やした測定ではない。

欠落経路は owner ごとに追加 rollout 本文 1 件を保存した seed DB を作り、追加 rollout を全 owner から削除し、残存 rollout に本文 1 件ずつ追記した。各 run の前に同じ seed に戻す。したがって元本文と欠落本文を保持しつつ更新し、全 owner が再保存される。

wall は Python monotonic clock で CLI 起動から終了まで。入力生成・seed 作成・DB 複製・検証は計時外。各条件 4 対、A/B・B/A の交互順、process を直列実行。各 process 30 秒、scratch 1 GiB を上限とし、実測最長 16.1 秒、scratch は確認した時点で最大 171 MiB。RSS は計測していない。新しい常駐状態や cache は追加していないが、メモリ改善を主張しない。

全 run 終了 0・stderr 空。初回は N ファイル、欠落更新も N ファイルの正常取込を確認。sessions / messages / import_state / legacy_sessions / legacy_messages の全保存列を比較し、実行日時の `imported_at` だけ除外した digest と stdout が各条件の両版で一致した。入力 path と ID は両版で同じため除外していない。元 snapshot は固定で変更しない。移行の失敗時保持・旧履歴保持は既存 fixture tests で別途確認した。

## 操作全体の wall

秒、中央値（最小–最大）、各版 4 回。

| owner × 本文 | 操作 | baseline | candidate |
|---|---|---:|---:|
| 4 × 8 | 初回 import・clean 再測定 | .0252 (.0243–.0261) | .0250 (.0247–.0263) |
| 4 × 8 | migrate・clean 再測定 | .0259 (.0249–.0262) | .0257 (.0250–.0263) |
| 4 × 8 | 欠落＋追記 | .0250 (.0247–.0255) | .0249 (.0244–.0253) |
| 64 × 64 | 初回 import | .3447 (.3379–.3707) | .3136 (.3091–.3183) |
| 64 × 64 | migrate | .3576 (.3506–.3598) | .3265 (.3209–.3381) |
| 64 × 64 | 欠落＋追記 | .5295 (.5235–.5585) | .3719 (.3605–.3788) |
| 256 × 64 | 初回 import・clean 再測定 | 1.9238 (1.9122–2.1182) | 1.3108 (1.2693–1.3481) |
| 256 × 64 | migrate・clean 再測定 | 1.9849 (1.9046–2.0308) | 1.2667 (1.2416–1.3325) |
| 256 × 64 | 欠落＋追記 | 4.0745 (4.0229–4.1730) | 1.4432 (1.4050–1.4825) |
| 256 × 256 | 初回 import | 6.7628 (6.6424–7.9826) | 4.2666 (4.2490–4.2753) |
| 256 × 256 | migrate | 6.8130 (6.7395–8.3241) | 4.3008 (4.2842–6.4058) |
| 256 × 256 | 欠落＋追記 | 15.1802 (15.1054–16.0913) | 4.8981 (4.7925–5.0985) |

初回の 4×8 import は .0252 (.0248–.0437) → .0245 (.0237–.4818)、migrate は .0248 (.0237–.0261) → .0270 (.0248–.0298)。小規模計測中に EQP helper の Go compile を同時に実行してしまったため、影響を受け得る初回値を採否に用いず両版を clean 再測定した。初回 256×64 import は 1.8775 (1.8457–1.9987) → 1.2663 (1.2317–1.2933)、migrate は 4.4776 (1.9068–12.3750) → 2.3657 (1.2804–9.1173)。後者には原因未特定の大きなばらつきがあり、その全値を残した上で bounded な 4 対を再測定した。初回に試した欠落 owner 全体削除だけのケースは本文 hash による skip となり保持 SELECT を測れなかったため、上記の同 owner 欠落 rollout＋追記へ両版の手順を変えた。

64 owner 以上で正常な同じ仕事の操作全体時間が分布の重なりを超えて短縮した。256×256 の初回 import / migrate は約 37%、欠落＋追記は約 68% 短縮。clean 小規模では測定可能な改善・悪化なし。既存索引に問い合わせを合わせるだけの変更で効果が得られたため採用する。64/256 owner と異なる payload・実ログ・cold disk・他機種の性能は未確認。`--full` は入力全削除後に保存し owner の ReplaceSession を通らないので、この DELETE 改善の対象ではない。保存形式は同じで、既存 DB の再取込は不要。

## Query plan と正しさ

project の SQLite engine で、本文 DELETE と保持 SELECT の旧条件は `SEARCH messages USING INDEX messages_session_idx (input_id=? AND source=?)`、新条件は `(input_id=? AND source=? AND identity=?)`。会話 DELETE / metadata SELECT は旧条件が `sqlite_autoindex_sessions_1 (input_id=? AND source=?)`、新条件が `sqlite_autoindex_sessions_2 (input_id=? AND identity=?)`。前者は全入力の本文を候補として owner 数だけ繰り返し判定するのに対し、新条件は owner を索引で限定する。追加 state や schema migration は不要だった。

変更前に `go test -count=1 ./internal/core ./internal/ingest/codex`（終了0、core 5.995秒 / Codex .329秒）。変更後に同コマンド（終了0、5.427秒 / .101秒）。既存 tests の input isolation、body/context/unresolved、欠落本文・parent・metadata、full、移行初回コピー・同 snapshot 再実行・拒否・本人 ID 置換・rollback・元 bytes/sidecar 不変を利用した。追加 test は `<`, `&`, quote, backslash を持つ owner ID を通常取込して同 owner rollout を欠落させ、残存本文を更新し、更新本文・欠落本文・保存metadataが残ることを確認する。特殊文字の identity 符号化と変更された SELECT/DELETE の実保存動作をまとめて検証し、SQL文字列や plan への恒久 test は追加しない。

共通 gate は Go format check（出力空、終了0、.16秒）、`go test -count=1 ./...`（終了0、core 5.831秒、CLI 2.343秒）、`go vet ./...`（終了0、.073秒）、`go build -o bin/somniloq ./cmd/somniloq`（終了0、.258秒）、指定版 `go install github.com/google/go-licenses/v2@v2.0.1`（権限拡張、終了0、.662秒）、`python3 scripts/update-third-party-notices.py --check`（終了0）。変更 Go は goimports 済み。一部通常 sandbox build は module version stat cache 書込警告を出したが、build と notices check は成功した。

## 再実行

[helper](codex-owner-scan-measure.py) と [全採否 sample](codex-owner-scan-results.jsonl) は生成時点の cache。実行は repository root、ログ・DB・binary は `.tmp/codex-owner-scan/` のみを再作成する。実データを指定しない。各 process は30秒 timeout。基準版を archive し、候補版は同 archive に今回の2ファイルだけ上書きしてビルドする。

```sh
mkdir -p .tmp/codex-owner-scan/base-source .tmp/codex-owner-scan/candidate-source
git archive 9df3e19b5edba0bae82c4b20baeb504ea0431cbd | tar -x -C .tmp/codex-owner-scan/base-source
git archive 9df3e19b5edba0bae82c4b20baeb504ea0431cbd | tar -x -C .tmp/codex-owner-scan/candidate-source
cp internal/core/db_write.go internal/core/import_codex.go .tmp/codex-owner-scan/candidate-source/internal/core/
(cd .tmp/codex-owner-scan/base-source && go build -o ../baseline ./cmd/somniloq)
(cd .tmp/codex-owner-scan/candidate-source && go build -o ../candidate ./cmd/somniloq)
python3 cache/codex-owner-scan-measure.py > .tmp/codex-owner-scan/results.jsonl
NO_MISSING=1 SIZES='[[4,8],[256,64]]' python3 cache/codex-owner-scan-measure.py > .tmp/codex-owner-scan/recheck-results.jsonl
cp cache/codex-owner-scan-eqp.go.txt .tmp/codex-owner-scan/eqp.go
go run .tmp/codex-owner-scan/eqp.go .tmp/codex-owner-scan/seed.db
```

`MISSING_ONLY=1` は保持経路だけを測る。`SIZES` は `[[owner数, ownerあたり本文数], ...]`、`MODES='["migrate"]'` は migrate のみ。helper は初回比較後に欠落＋追記を測り、全保存値を検証する。全実行の wall と stdout、digest が JSONL に出る。再測定ごとの初期 DB 構築・入力生成は計時外である。
