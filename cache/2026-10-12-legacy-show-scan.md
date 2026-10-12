---
generated: 2026-10-12
baseline_commit: 6bee6d393b4f33ddf0f2f6b4c630248f191e1571
candidate: この記録と同じ差分の show.go と db_messages_summary.go
---

# legacy show の会話本文探索

採用: 選択済み SessionRow を count / 本文取得へ渡し、legacy REF の snapshot digest を SQL に bind する。既存の `legacy_messages_session_idx(snapshot_sha256,source,session_id)` に問い合わせを揃え、schema は変えない。親子関係や探索対象は追加しない。

## 計測条件

Darwin arm64、Go 1.27.1、通常 build、modernc.org/sqlite v1.29.0（同driverで `SELECT sqlite_version()` を実行してSQLite 3.45.1を確認）。実 schema を core.OpenDB で作成し、合成 legacy Codex 会話を各20本文、本文128 bytes、UUID重複なしで保存した。総本文1,000件（50会話）と100,000件（5,000会話、DB約36MiB）に対し、先頭1 / 8 REFを選択した。通常sourceの20本文・1 REFも比較した。

DB生成後の温かいOS cacheで、before / after を交互に各6回、処理を直列実行した。局所計測は同じGo SQLite driverで count と全本文のquery / scan完了までを測り、CLIはプロセス起動からJSONファイルへの出力完了・終了までのwall、CPU、wait4 peak RSSを測った。EXPLAINは局所計時に含めない。予算はDB100MiB、1 CLI process 10秒、全測定5分。DB計測用の見積りは2つのseed DBで約37MiB、binary・出力を含む計測用scratchは実測約88MiB（workspace内Go build cache約280MiBは別）。正式CLIの最大wallは125.19ms、最大peak RSSは18.89MiBで上限到達なし。全180 CLI runでexit 0、totalとcountが期待どおり、条件ごとのstdout SHA-256が両版で一致した。CLIの初回候補起動に370msの外れ値があった予備runは正式比較と混ぜず、両binaryのhelpを先に起動した後で全条件を対比較し直した。

## Query plan と局所時間

1,000 / 100,000件とも、count / 本文取得は `SCAN legacy_messages` から `SEARCH legacy_messages USING INDEX legacy_messages_session_idx (snapshot_sha256=? AND source=? AND session_id=?)` へ変わった。本文の `USE TEMP B-TREE FOR ORDER BY` は両版で残る。number順を変える追加最適化は行わない。

count＋全件本文取得のwall、単位ms、中央値（最小–最大）:

| 総本文 | 選択REF | before | after |
|---:|---:|---:|---:|
| 1,000 | 1 | 0.212 (0.209–0.269) | 0.068 (0.066–0.076) |
| 1,000 | 8 | 1.756 (1.705–1.808) | 0.557 (0.534–0.965) |
| 100,000 | 1 | 25.255 (24.792–25.339) | 0.099 (0.076–0.183) |
| 100,000 | 8 | 201.189 (200.241–202.513) | 0.610 (0.552–0.665) |

この区間では全REFのcountと本文の両方を取得するため、全件CLI（countなし）やlimit/tail CLI（すべてのcountと1会話の本文のみ）と処理量が異なる。局所値とCLI値を引き算して段階時間を算出しない。全履歴規模と選択REF数に伴う全体走査の増大をplanと実測の両方で確認した。

## CLI総時間

単位ms、中央値（最小–最大）。fullは全件、limit / tailは5件、normalは通常sourceの対照。

| 総本文 | 選択REF | 操作 | before | after |
|---:|---:|---|---:|---:|
| 1000 | 1 | full | 8.89 (8.63–10.06) | 8.77 (7.60–9.02) |
| 1000 | 1 | limit | 8.46 (7.48–9.08) | 8.59 (7.89–8.84) |
| 1000 | 1 | tail | 8.87 (8.40–9.17) | 8.60 (7.76–8.82) |
| 1000 | 8 | full | 9.78 (9.46–10.28) | 8.86 (8.51–9.71) |
| 1000 | 8 | limit | 9.79 (8.76–9.85) | 8.87 (8.25–9.11) |
| 1000 | 8 | tail | 9.53 (8.67–10.31) | 8.75 (8.31–9.55) |
| 100000 | 1 | full | 21.27 (20.93–22.34) | 8.67 (8.11–8.79) |
| 100000 | 1 | limit | 33.70 (33.38–34.11) | 8.56 (7.75–9.20) |
| 100000 | 1 | tail | 34.25 (33.76–35.07) | 8.70 (7.87–8.91) |
| 100000 | 8 | full | 110.96 (110.25–112.17) | 9.17 (8.67–9.94) |
| 100000 | 8 | limit | 122.39 (122.00–122.88) | 8.77 (8.43–9.88) |
| 100000 | 8 | tail | 123.36 (122.16–125.19) | 8.73 (8.47–8.83) |
| normal | 1 | full | 8.54 (7.67–8.81) | 8.74 (8.67–8.91) |
| normal | 1 | limit | 8.76 (7.66–8.89) | 8.62 (7.77–9.04) |
| normal | 1 | tail | 8.68 (8.38–8.88) | 8.80 (7.60–9.11) |

100,000件・8 REFではfullが110.96→9.17ms、limitが122.39→8.77ms、tailが123.36→8.73msで、各6回の範囲は分離した。同条件のCPU中央値はfull 111.54→8.65ms、limit 121.89→8.15ms、tail 122.57→8.21ms。peak RSS中央値はfull 18.41→15.05MiB、limit 17.76→14.45MiB、tail 17.74→14.45MiB。新しいcacheや保存状態は追加していない。

1,000件1 REFと通常source対照のwall差はばらつき内で、明確な改善・悪化は確認できない。1,000件8 REFの差も約1msで範囲が重なる。大きいlegacy履歴で局所時間と利用者が待つCLI総時間の両方が大幅に減り、変更の負担は選択済みsessionとREFの利用に収まるため採用した。

## 正しさと検証

基準版のcore / CLIテストを変更前に実行し成功。追加のCLI回帰テストはlegacyの複数REF、指定順と重複除去、number順、REFをまたぐoffset / limit、tail、role・番号条件、Cursor sidechain除外、非選択会話を確認する。既存の通常source、legacy原文・provenance、日時条件のテストも成功した。実binaryの通常source少量表示（stdout JSON、stderr空、exit 0）と不正REF（stdout空、stderr診断、exit 2）も両版で確認した。CLI表示形式・flags・schema・保存形式は変えない。

共通gate: goimports、format空、`go test -count=1 ./...`（6.17秒）、`go vet ./...`（0.23秒）、`go build -o bin/somniloq ./cmd/somniloq`（0.34秒）、`go install github.com/google/go-licenses/v2@v2.0.1`、通知checkはいずれもexit 0。sandboxのGo cache書込制限はworkspace内GOCACHEで回避し、go-licensesのDNS拒否は権限拡張で解消した。buildにはmodule stat cache書込警告が出たがexit 0、binaryの実行比較は成功した。

限界: 合成した固定20本文の会話、温かいcache、単一機種・driverの結果。cold disk、非常に大きい選択会話、100,000件超、並行アクセスは未計測。局所計測はproductionと同じSQLをhelperで実行し、CLI測定は実binaryのAPI・REF解析・session解決・表示・起動を含む。局所のメモリ割当や物理I/Oは計測していない。速さの絶対値を未測定環境へ外挿しない。

## 再実行

生成時点のhelperは `cache/legacy-show-measure.go.txt`（実schema作成、seed、plan、局所対比較）と `cache/legacy-show-measure.py`（CLI対比較）。前者は20本文の通常source対照も作る。専用一時DBを再作成するため、計測中のDBを他用途に使わない。基準sourceは作業treeを切り替えず隔離する。

```sh
mkdir -p .tmp/legacy-show/baseline-source
git archive 6bee6d393b4f33ddf0f2f6b4c630248f191e1571 | tar -x -C .tmp/legacy-show/baseline-source
(cd .tmp/legacy-show/baseline-source && go build -o ../before ./cmd/somniloq)
go build -o .tmp/legacy-show/after ./cmd/somniloq
cp cache/legacy-show-measure.go.txt .tmp/legacy-show/measurement.go
go run .tmp/legacy-show/measurement.go > .tmp/legacy-show/retrieval.jsonl 2> .tmp/legacy-show/plans.txt
python3 cache/legacy-show-measure.py > .tmp/legacy-show/cli.jsonl
```

JSONLは各回のms・件数・side・規模・REF数を含み、CLIはCPU / peak RSS / stdout hashも残す。集計は条件・sideごとの6回の中央値と最小・最大。日時不明のlegacy timestampは補完せず、元のnumberを維持する。
