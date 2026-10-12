---
generated: 2026-10-12
baseline_commit: 6610e4e8fb1228987b45a286dbd866e4ab20e83b
candidate: この記録と同じ差分の show.go、db_sessions_projects.go、db_legacy.go、session_relations.go
---

# 単一 REF の show の関係取得

採用: session metadata と本文集約の SELECT を分離し、show に metadata 専用 Resolver を使う。Self lookup と namespace query の両方で本文件数・byte数の集約を省く。通常 Resolver、LookupSessionREF、ListSessions の集計値を維持する。metadata 専用 Resolver は返す SessionRow の MessageCount・BodySize を未取得のゼロとし、その契約を API コメントに置く。graph の取得範囲・構築・親子関係・診断、read snapshot 内の cache 寿命は変えない。

## 測定条件

macOS Darwin arm64、Go 1.27.1、通常 build、modernc.org/sqlite v1.29.0。対象は合成 Codex 1会話・4発言。同じ入力・source に64個の無関係な会話を置き、そこへ4 KiB本文を0 / 1,000 / 10,000件保存する。target は baseline CLI で JSONL を取り込み、無関係な行は同じ現行schemaに Python sqlite3 で追加する。本文とblocks_jsonを両方保存し、最大DBは約84 MiB。全条件で65会話に固定し、無関係な本文量だけを変える。

プロセスを直列に baseline / candidate の順を交互に各8対実行した。wall は起動から終了まで、CPU と peak RSS は macOS wait4 の値。JSON はファイルへ出力。1 process 10秒でtimeout、RSSは終了後512 MiB未満を確認する。正式比較192 run の最大wallは698.82 ms、最大RSSは33.06 MiB。OS cacheは先行実行で温まるが、DBごとの最初のbaseline fullに大きな外れ値が残るため、中央値と全範囲を載せる。cold disk の保証はしない。

## CLI 結果

単一REFは4発言全件または `--limit 1`。複数REFはtargetとunrelated-0を指定し、後者の発言数も入力規模に伴って増える。wallはms、RSSはMiB、中央値（最小–最大）。

| 無関係な本文 | 操作 | baseline wall | candidate wall | baseline RSS | candidate RSS |
|---:|---|---:|---:|---:|---:|
| 0 | 単一・全件 | 9.33 (8.88–30.72) | 9.52 (8.85–29.68) | 14.28 (13.83–14.38) | 14.24 (13.80–14.33) |
| 1,000 | 単一・全件 | 11.41 (11.07–75.28) | 8.89 (8.68–9.63) | 17.53 (16.94–17.61) | 14.10 (13.80–14.27) |
| 10,000 | 単一・全件 | 34.75 (33.92–698.82) | 8.99 (8.71–10.01) | 17.53 (17.22–17.75) | 14.11 (13.91–14.33) |
| 10,000 | 単一・limit 1 | 34.06 (33.47–35.48) | 9.04 (8.56–9.80) | 17.65 (17.59–17.80) | 14.18 (13.97–14.34) |
| 10,000 | 複数・全件 | 41.47 (39.14–42.23) | 15.69 (15.17–16.98) | 31.81 (30.73–33.06) | 31.82 (31.31–33.06) |

単一・全件10,000件のCPU中央値は33.71→8.33 ms。無関係な本文0件の limit と複数REFは wall 約9 ms・RSS約14 MiBで両版同程度。不正REFは全規模でexit 2、wall中央値8.81–9.12 / 8.89–8.97 ms、RSS約13.5 MiBで同程度。小規模で明確な悪化は観測されなかったが、sub-msの差は確定できない。

最初の比較は共通gateと同時に走らせたため正式比較と混ぜない。その後、同じDBを直列で再比較し、単一・全件10,000件は39.47→10.23 msだった。計測helperへtotal/countの期待値検査を加えて再実行した結果を上表とする。これらの前段比較でも、全件10,000件のwallは約35–39→9–10 ms、RSSは約17.5→14.1 MiBで改善の方向は一致した。

## SQL 負担

同じGo SQLite driverでnamespace取得SQLの `EXPLAIN QUERY PLAN` を確認した。baselineにあった次の探索がcandidateから消え、inputs・sessions・parent・rootの探索だけが残った。

```text
SEARCH m USING INDEX messages_session_idx (input_id=? AND source=? AND identity=?) LEFT-JOIN
```

10,000件DBでnamespace query実行から65行のscan完了までを交互8対で計測した。baseline 25.10 (24.18–27.86) ms、candidate 0.31 (0.23–0.36) ms。計測範囲はQuery＋全行Scanでありgraph構築・Self lookup・CLI起動を含まない。物理I/O bytesやSQLite VM命令数は測っていない。

局所測定の導出手順: baselineの `sessionRowSelect` とcandidateの `sessionMetadataSelect` に `WHERE s.input_id=? AND s.source=? GROUP BY s.input_id,s.source,s.identity` を付け、seed DBのinput_id=1、source=codexで実行する。database/sql＋modernc.org/sqlite v1.29.0でQuery、Rows.Next、全列Scan、Rows.Err、Rows.Closeまでをtime.Now/time.Sinceで測り、両版の順を交互に8回ずつ実行する。EXPLAINは計時外に実行する。

## 正しさと限界

正式192 runすべてで対ごとのstdout SHA-256、stderr、終了値が一致。成功系はexit 0、単一全件total/count=4、limitはtotal=4/count=1、複数は4＋unrelated-0の件数を検査した。不正REFはexit 2。

core回帰テストでmetadataと通常ResolverのSelf・Parent・Root・Members・Descendants・診断を比較し、通常APIの件数とUTF-8 byte値は実際の本文で検査した。Codex・Claude child/未確定子・Cursor・legacy、descendantsの有無と循環を確認した。既存CLIテストでページ・日時・複数REF・欠落親・元本文・legacyを確認した。goimports、format、全体test、vet、build、指定版go-licenses install、通知checkは終了0。通知checkには既存のassembly依存解析とsandboxのmodule stat cache書き込み警告が出たが、checkは成功した。

namespace全会話のmetadata読込みとgraph構築は残る。会話数自体の増大、巨大な表示対象、cold disk、他機種は性能未計測。複数全件では選択した本文のdecode・出力が残るためピークRSSは改善しなかった。性能採用は今回の本文集約分離に限る。

## CLI 比較の再実行

repository rootで実行する。helperは `.tmp/show-relations/` のseed DBを作り直すため、そこに保存した作業用データを必要なら退避する。

```sh
mkdir -p .tmp/show-relations/baseline-source
git archive 6610e4e8fb1228987b45a286dbd866e4ab20e83b | tar -x -C .tmp/show-relations/baseline-source
(cd .tmp/show-relations/baseline-source && go build -o ../baseline ./cmd/somniloq)
go build -o .tmp/show-relations/candidate ./cmd/somniloq
python3 cache/show-relations-measure.py seed
python3 cache/show-relations-measure.py measure > .tmp/show-relations/summary.jsonl
```

helperは各runのwall・CPU・RSS・出力hash・終了値・total/countを `.tmp/show-relations/paired.json` に、集計をstdoutへ出す。生成時点の候補差分を比較するhelperであり、将来の候補の性能・動作を保証しない。
