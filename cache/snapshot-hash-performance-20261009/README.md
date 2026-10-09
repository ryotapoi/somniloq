---
observed_at: 2026-10-09
compiled_from_commit: 1f9cccb972983bd2ad64158a2ef6aa5f50593c71
---

# migration の rollout snapshot hash 読み込み

`checkMigrationSnapshot` が各 rollout の全 bytes を `os.ReadFile` で再確保していた箇所を、32 KiB bufferを再利用する逐次SHA-256計算に変更して採用した。最大1ファイル約64.2 MiBの単一会話では初回CLIピークRSS中央値355.55→292.20 MiB（63.35 MiB、17.8%減）、同じsnapshotの再実行では351.17→287.27 MiB（63.90 MiB、18.2%減）。各3回の範囲は重ならない。時間短縮は確認できなかった。file集合と各fileの全bytes hashは、置換前およびtransactionのcommit前に引き続き確認する。読み取り失敗はpath付きで返し、該当groupの既存本文・旧行・cursorを保持する。保存形式とschemaは変わらない。

`io.CopyBuffer` は `*os.File.WriteTo` を優先すると渡したbufferを使わないため、読み取り専用のinterfaceに包んで `WriteTo` を隠した。最初の候補も全文再確保を避けてRSSは下がったが、buffer再利用が成立していなかったため修正し、最終候補で全比較をやり直した。[初回候補の生値](preliminary-results.json)は採否には使っていない。

## 条件と結果

基準は上記commitのclean source、候補は `internal/core/migrate.go` の局所差分を含む通常build。macOS 27.0.1 / 26A434、Go 1.27.1 darwin/arm64、`GOFLAGS=-buildvcs=false`、同じ専用Go cache。既存の[入力生成・予算監視・保存検証](../migration-performance-20261008/measure.py)を再利用した。入力生成・build・DB検査は計時外。`/usr/bin/time -l` のCLI起動から完了までの時間、user+sys CPU、processピークRSSを測定した。filesystem cacheと同居負荷は無制御で、bench/testとの同時実行なし。各版の初回は移行先DBなし、再実行はその版が直前に作ったDBと同じsnapshot。各操作60秒、CLI RSS 1 GiB、専用領域2 GiBを上限に250 ms間隔で子processを監視した。中断なし。

各入力で基準→候補、候補→基準、基準→候補の順に3組、各版の初回と再実行を連続実行した。全36操作exit 0。値は中央値 [最小–最大]、秒とMiB（1 MiB=1,048,576 bytes）。外れ値は除外していない。

| 入力・操作 | 版 | 時間 秒 | CPU 秒 | RSS MiB |
| --- | --- | ---: | ---: | ---: |
| 単一会話、初回 | 基準 | 0.81 [0.76–0.86] | 0.81 [0.76–0.84] | 355.55 [355.34–356.22] |
| 単一会話、初回 | 候補 | 0.81 [0.76–1.08] | 0.80 [0.76–0.81] | 292.20 [290.88–292.30] |
| 単一会話、再実行 | 基準 | 0.96 [0.95–0.98] | 0.93 [0.93–0.95] | 351.17 [351.06–351.61] |
| 単一会話、再実行 | 候補 | 0.98 [0.95–0.99] | 0.96 [0.93–0.97] | 287.27 [287.20–287.30] |
| 16会話、64 MiB、初回 | 基準 | 0.78 [0.78–0.80] | 0.88 [0.87–0.89] | 63.67 [59.52–63.98] |
| 16会話、64 MiB、初回 | 候補 | 0.77 [0.77–0.78] | 0.84 [0.84–0.86] | 58.27 [58.20–59.09] |
| 16会話、64 MiB、再実行 | 基準 | 0.98 [0.97–1.03] | 1.05 [1.04–1.11] | 56.73 [54.89–59.30] |
| 16会話、64 MiB、再実行 | 候補 | 1.00 [0.96–1.02] | 1.06 [1.01–1.07] | 54.23 [53.23–54.48] |
| 16会話、短本文、初回 | 基準 | 0.12 [0.11–0.12] | 0.12 [0.11–0.13] | 30.98 [30.58–31.45] |
| 16会話、短本文、初回 | 候補 | 0.12 [0.12–0.12] | 0.12 [0.12–0.13] | 30.88 [30.86–31.41] |
| 16会話、短本文、再実行 | 基準 | 0.12 [0.11–0.12] | 0.11 [0.11–0.11] | 30.83 [30.61–31.33] |
| 16会話、短本文、再実行 | 候補 | 0.12 [0.12–0.12] | 0.11 [0.11–0.11] | 30.94 [30.42–31.09] |

単一・16分割はいずれも本文総量64 MiB、1,024件。前者は1ファイル・最大会話約64.2 MiB、後者は16ファイル・各64件・最大ファイル約4.0 MiB。短本文は16会話・1,024件で各512 bytes、入力総量約0.66 MiB。単一会話では削減量が読み直していた最大ファイルの大きさに近く、原因仮説と整合する。16分割初回もRSSの範囲は重ならず、中央値で5.40 MiB減った。再実行・短本文では測定可能な差と断定しない。実ログ、64 MiB超のfile、cold cache、他OSへは外挿しない。

## 正しさとgate

変更前 `go test -count=1 ./...` は専用Go cacheでexit 0（3.29秒）。候補のcore/codex testはexit 0（最終候補2.86秒）。`TestMigrateOwnerFailurePreservesPriorState/changed_rollout` はhash不一致、`.../missing_rollout` はfile集合不一致、今回追加した `.../unreadable_rollout` は集合にpathを残したままopenに失敗する場合に、既存本文・旧行・cursorを保持する。`TestMigrateMissingRolloutBeforeRetryPreservesPriorState` は同一snapshot再実行時の保持を確認する。読み込み途中で発生するread errorの注入は、この検証では行っていない。計測driverは各操作で正常group数、copy有無、snapshot digest、旧行削除/他source保持、保存会話/発言数・本文長/総量・role/membership/timestamp・cursor終端、SQLite integrity、移行元digest不変・sidecar不在を確認した。`imported_at` 以外の全destination table行のdigestも基準・候補の各ペアで一致した。`imported_at` は実行時刻なので比較から外した。

[共通gateの生値](gates.json)にcommand・exit code・durationを保存。最終候補でformat、`go test -count=1 ./...`、`go vet ./...`、`go build -o bin/somniloq ./cmd/somniloq`、通知checkはexit 0。`go install github.com/google/go-licenses/v2@v2.0.1` はsandboxのDNS制限で初回exit 1、監視可能な環境で再実行してexit 0。

## 再実行

[計測driver](measure.py)と[全操作の生値](results.json)を保存した。対象process終了後に自分の専用 `.tmp/snapshot-hash/inputs` だけを片付け、上記commitから基準を作る。候補は本変更の作業ツリーを通常buildする。

```sh
mkdir -p .tmp/snapshot-hash/baseline-source
git archive 1f9cccb972983bd2ad64158a2ef6aa5f50593c71 go.mod go.sum internal cmd | tar -x -C .tmp/snapshot-hash/baseline-source
(cd .tmp/snapshot-hash/baseline-source && GOCACHE="$OLDPWD/.tmp/migration-cumulative-performance/go-cache" GOFLAGS=-buildvcs=false go build -o "$OLDPWD/.tmp/snapshot-hash/baseline" ./cmd/somniloq)
GOCACHE="$PWD/.tmp/migration-cumulative-performance/go-cache" GOFLAGS=-buildvcs=false go build -o "$PWD/.tmp/snapshot-hash/candidate" ./cmd/somniloq
python3 cache/snapshot-hash-performance-20261009/measure.py
```

`pgrep`/`ps` を使える権限が必要。結果は `.tmp/snapshot-hash/results.json` に出る。
