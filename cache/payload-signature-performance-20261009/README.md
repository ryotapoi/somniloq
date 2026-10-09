---
observed_at: 2026-10-09
compiled_from_commit: 42e7db5a415f9ba5aa7f9e163018ed130455ed86
---

# payload ID 重複判定の全文署名除去

単一64 MiB会話の移行で、payload ID ごとに保持していた Role/Blocks/Timestamp の全文JSON署名を、既に保持する代表メッセージの位置に置き換えた。採用する。初回のピークRSS中央値は465.59→356.66 MiB（108.94 MiB、23.4%減）、同一snapshot再実行は461.69→353.00 MiB（108.69 MiB、23.5%減）。各3回の範囲も重ならない。操作全体の時間には揺れがあり、単一会話での速度改善は主張しない。

比較するのは旧署名と同じ role・timestamp・text block 列で、block の数・境界、nil と空sliceも区別する。代表位置は重複除去後の出力sliceを指し、in-place圧縮で上書きされない。payload IDのない本文は従来どおり番号を付け、context・unresolved は重複判定と番号付けの対象外。hashだけの一致判定、元ファイルbytesの保持、snapshot hash、GC設定には触れない。

## 条件と結果

変更前は上記commitのclean source、候補は `internal/ingest/codex/group.go` の署名除去と対応テストを加えた作業ツリー。macOS 27.0.1 / 26A434、Go 1.27.1 darwin/arm64、通常 `go build`、`GOFLAGS=-buildvcs=false`、同じ専用Go cache。既存の[入力生成・予算監視・保存検証](../migration-performance-20261008/measure.py)を使用した。入力生成・build・DB検査は計時外。`/usr/bin/time -l` によるCLI起動から完了までの時間とprocess単位ピークRSS。filesystem cacheと同居負荷は無制御で、他のbench/testは同時実行しない。各版の初回は移行先DBなし、再実行はその版の直前に作ったDBと同一snapshot。各操作60秒、CLI RSS 1 GiB、専用領域2 GiBを上限とし、250 msごとに子processを監視。中断なし。

各条件で基準→候補、候補→基準、基準→候補の順に3組、初回と再実行を連続実行した。全36操作exit 0。値は中央値 [最小–最大]。時間は秒、RSSはMiB（1 MiB=1,048,576 bytes）。外れ値は除外しない。CPUはuser+sys。

| 入力・操作 | 版 | 時間 秒 | CPU 秒 | RSS MiB |
| --- | --- | ---: | ---: | ---: |
| 単一会話、初回 | 基準 | 1.18 [0.97–1.20] | 1.08 [0.88–1.14] | 465.59 [463.06–466.56] |
| 単一会話、初回 | 候補 | 1.38 [0.86–1.39] | 1.03 [0.84–1.10] | 356.66 [355.61–357.64] |
| 単一会話、再実行 | 基準 | 1.20 [1.14–1.62] | 1.18 [1.10–1.37] | 461.69 [451.03–462.81] |
| 単一会話、再実行 | 候補 | 1.54 [1.09–1.72] | 1.33 [1.06–1.41] | 353.00 [351.23–354.03] |
| 16会話、64 MiB、初回 | 基準 | 0.93 [0.88–0.94] | 1.01 [0.97–1.03] | 64.72 [63.02–65.73] |
| 16会話、64 MiB、初回 | 候補 | 0.86 [0.84–0.87] | 0.93 [0.92–0.94] | 60.34 [58.30–63.45] |
| 16会話、64 MiB、再実行 | 基準 | 1.09 [1.08–1.12] | 1.18 [1.16–1.19] | 59.92 [56.59–60.95] |
| 16会話、64 MiB、再実行 | 候補 | 1.07 [1.06–1.08] | 1.14 [1.13–1.14] | 52.94 [51.67–53.28] |
| 16会話、短本文、初回 | 基準 | 0.13 [0.13–0.13] | 0.13 [0.13–0.13] | 31.14 [31.08–31.34] |
| 16会話、短本文、初回 | 候補 | 0.13 [0.12–0.15] | 0.13 [0.13–0.13] | 31.34 [31.22–31.78] |
| 16会話、短本文、再実行 | 基準 | 0.12 [0.12–0.12] | 0.13 [0.12–0.13] | 31.03 [30.81–31.20] |
| 16会話、短本文、再実行 | 候補 | 0.12 [0.11–0.14] | 0.12 [0.11–0.13] | 31.44 [30.69–31.61] |

単一・16分割はいずれも本文総量64 MiB、1,024件。前者は1ファイル・最大会話64 MiB、後者は16ファイル・各64件。短本文は同じ16会話・1,024件で各512 bytes、入力総量約0.66 MiB。16分割のRSSと初回時間は改善傾向だが、短本文のRSSと時間に測定可能な改善はない。単一会話の初回時間中央値は上がり、範囲が重なる。再実行の時間も範囲が重なる。主対象のRSS削減は大きく安定しており、mapには小さいindex以外の状態を増やさないので採用した。64 MiBを超える会話、実ログ、cold cache、他OSの効果には外挿しない。

## 正しさとgate

変更前の `go test -count=1 ./...` はexit 0（約2.83秒）。変更後は同一payload IDの重複除去、最初の行の保持、in-place圧縮後の連番を通常import・migrationの両方で確認した。正確比較のrole・timestamp・block境界・本文・nil/空sliceも恒久unit testで確認した。既存fixtureはpayload不一致衝突、埋め込み親・本人境界、context・unresolvedの除外、物理行、拒否、失敗時保持、同一snapshot再実行を覆う。測定harnessは正常group数・copy有無・旧行削除/他source保持・保存会話/発言数・本文長/総量・role/membership/timestamp・cursor終端・SQLite integrity・移行元digest不変・sidecar不在を各操作で確認した。保存形式とschemaは変更しないため、既存DBの再取り込み案内は不要。

[共通gate生値](gates.json)にコマンド・終了コード・時間を保存。最終的に `goimports -w`、format空確認、`go test -count=1 ./...`、`go vet ./...`、`go build -o bin/somniloq ./cmd/somniloq`、`go install github.com/google/go-licenses/v2@v2.0.1`、`python3 scripts/update-third-party-notices.py --check` はすべてexit 0。`go install` はsandboxのDNS制限で初回exit 1、監視可能な環境で再実行して成功した。

## 再実行

[計測driver](measure.py)と[全操作の生値](results.json)を保存した。再実行時は自分のprocess終了を確かめ、専用 `.tmp/payload-signature` だけを除去する。今回の差分を含む作業ツリーで、リポジトリrootから次を実行する。

```sh
mkdir -p .tmp/payload-signature/baseline-source
git archive 42e7db5a415f9ba5aa7f9e163018ed130455ed86 go.mod go.sum internal cmd | tar -x -C .tmp/payload-signature/baseline-source
GOCACHE="$PWD/.tmp/migration-cumulative-performance/go-cache" GOFLAGS=-buildvcs=false go build -o "$PWD/.tmp/payload-signature/candidate" ./cmd/somniloq
(cd .tmp/payload-signature/baseline-source && GOCACHE="$OLDPWD/.tmp/migration-cumulative-performance/go-cache" GOFLAGS=-buildvcs=false go build -o "$OLDPWD/.tmp/payload-signature/baseline" ./cmd/somniloq)
python3 cache/payload-signature-performance-20261009/measure.py
```

`pgrep`/`ps` が使える権限が必要。Go cacheは `.tmp/migration-cumulative-performance/go-cache` を再利用する。結果は `.tmp/payload-signature/results.json` に書かれ、公開用の `cache/` への取り込みは手動で行う。
