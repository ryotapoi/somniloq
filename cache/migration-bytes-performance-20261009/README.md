---
observed_at: 2026-10-09
compiled_from_commit: 1c3698b1acd8ca3da6fc28b1ffa972fbaf33f122
---

# migration 本文 pass の元ファイル bytes 保持

`BuildMigrationGroups` の `FileReport.Data` 保持を省く候補を評価し、採用を見送った。単一64 MiB会話の初回ピークRSS中央値は355.50→356.22 MiB、同一snapshot再実行は351.20→351.14 MiBで、改善がない。16会話に分けた64 MiBでは初回62.03→58.06 MiB、再実行55.20→52.77 MiBと下がる傾向はあるが、各3回の範囲が重なり、対の初回1組では候補の方が高い。短本文にも改善がない。時間は単一会話の初回0.85→0.90秒、再実行1.06→1.09秒で範囲が重なり、速度改善も主張できない。候補の1行変更は戻し、製品コードは変更していない。

通常importの `FileReport.Diagnostics` は `Data` で既存prefixのhashと未終端行を判定するため、その保持は維持した。migrationではpath・全ファイルhash・物理行・cursor・所属証拠を従来どおり作り、本文 pass の `Data` だけを保持しない[候補差分](candidate.patch)を測った。単一巨大会話では、全文を読む処理中の一時 bytes や正規化本文などがピークを決め、reportへの参照を減らすだけではピークを下げられなかった可能性がある。どの割当が実際にピークを決めたかは、この計測では特定していない。

## 条件と結果

基準は上記commitのclean source、候補は同commitに1行のpatchを適用した通常build。macOS 27.0.1 / 26A434、Go 1.27.1 darwin/arm64、`GOFLAGS=-buildvcs=false`、同じ専用Go cache。前タスクと同じ[入力生成・予算監視・保存検証](../migration-performance-20261008/measure.py)を使い、入力生成・build・DB検査は計時外。`/usr/bin/time -l` によるCLI起動から完了までの時間とprocess単位ピークRSSを測定した。filesystem cacheと同居負荷は無制御、bench/testとの同時実行なし。各版の初回は移行先DBなし、再実行はその版が直前に作ったDBと同一snapshot。各操作60秒、CLI RSS 1 GiB、専用領域2 GiBを上限に、250 msごとに子processを監視した。中断なし。

各条件で基準→候補、候補→基準、基準→候補の順に3組、各版の初回と再実行を連続実行した。全36操作exit 0。値は中央値 [最小–最大]。時間は秒、RSSはMiB（1 MiB=1,048,576 bytes）。外れ値は除外しない。

| 入力・操作 | 版 | 時間 秒 | RSS MiB |
| --- | --- | ---: | ---: |
| 単一会話、初回 | 基準 | 0.85 [0.84–0.89] | 355.50 [355.45–356.33] |
| 単一会話、初回 | 候補 | 0.90 [0.84–1.26] | 356.22 [355.89–356.30] |
| 単一会話、再実行 | 基準 | 1.06 [1.05–1.09] | 351.20 [351.05–351.63] |
| 単一会話、再実行 | 候補 | 1.09 [1.04–1.13] | 351.14 [350.67–352.50] |
| 16会話、64 MiB、初回 | 基準 | 0.89 [0.88–0.91] | 62.03 [58.62–64.25] |
| 16会話、64 MiB、初回 | 候補 | 0.89 [0.86–0.89] | 58.06 [57.23–59.88] |
| 16会話、64 MiB、再実行 | 基準 | 1.12 [1.07–1.13] | 55.20 [53.17–59.30] |
| 16会話、64 MiB、再実行 | 候補 | 1.14 [1.09–1.14] | 52.77 [51.11–55.11] |
| 16会話、短本文、初回 | 基準 | 0.13 [0.13–0.14] | 31.75 [31.23–31.77] |
| 16会話、短本文、初回 | 候補 | 0.13 [0.13–0.14] | 31.36 [31.22–31.88] |
| 16会話、短本文、再実行 | 基準 | 0.13 [0.13–0.13] | 30.81 [30.78–30.95] |
| 16会話、短本文、再実行 | 候補 | 0.13 [0.12–0.13] | 32.08 [30.42–32.09] |

単一・16分割はいずれも本文総量64 MiB、1,024件。前者は1ファイル・最大会話64 MiB、後者は16ファイル・各64件。短本文は16会話・1,024件で各512 bytes、入力総量約0.66 MiB。測定結果はこれらの合成入力と環境に限る。64 MiBを超える会話、実ログ、cold cache、他OSには外挿しない。

## 正しさとgate

変更前の `go test -count=1 ./...` はexit 0（3.26秒、専用Go cache）。候補ではmigration reportの `Data` がnilであることを一時testで確認し、既存の通常import診断と移行fixtureを含む `go test -count=1 ./internal/ingest/codex ./internal/core` はexit 0（3.21秒）。候補の保存形式・schema・通常import経路は変更していない。測定harnessは正常group数・copy有無・旧行削除/他source保持・保存会話/発言数・本文長/総量・role/membership/timestamp・cursor終端・SQLite integrity・移行元digest不変・sidecar不在を各操作で確認した。既存fixtureは拒否・失敗時保持と再実行も覆う。製品コードを戻した後の[共通gate生値](gates.json)ではformat、全test、vet、build、ライセンス通知checkがexit 0。`go install` はsandboxのDNS制限で初回exit 1、監視可能な環境で再実行してexit 0。

## 再実行

[計測driver](measure.py)と[全操作の生値](results.json)、[候補差分](candidate.patch)を保存した。専用 `.tmp/migration-bytes` 内の前回の `inputs` を片付けたうえで、上記commitから基準・候補を作る。どちらも通常の `go build` を使う。

```sh
mkdir -p .tmp/migration-bytes/baseline-source .tmp/migration-bytes/candidate-source
git archive 1c3698b1acd8ca3da6fc28b1ffa972fbaf33f122 go.mod go.sum internal cmd | tar -x -C .tmp/migration-bytes/baseline-source
cp -R .tmp/migration-bytes/baseline-source/. .tmp/migration-bytes/candidate-source/
patch -d .tmp/migration-bytes/candidate-source -p1 < cache/migration-bytes-performance-20261009/candidate.patch
(cd .tmp/migration-bytes/baseline-source && GOCACHE="$OLDPWD/.tmp/migration-cumulative-performance/go-cache" GOFLAGS=-buildvcs=false go build -o "$OLDPWD/.tmp/migration-bytes/baseline" ./cmd/somniloq)
(cd .tmp/migration-bytes/candidate-source && GOCACHE="$OLDPWD/.tmp/migration-cumulative-performance/go-cache" GOFLAGS=-buildvcs=false go build -o "$OLDPWD/.tmp/migration-bytes/candidate" ./cmd/somniloq)
python3 cache/migration-bytes-performance-20261009/measure.py
```

`pgrep`/`ps` が使える権限が必要。入力とDBは `.tmp/migration-bytes/inputs`、再計測の生値は `.tmp/migration-bytes/results.json` に出る。
