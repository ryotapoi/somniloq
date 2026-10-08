---
observed_at: 2026-10-09
compiled_from_commit: 3b0107eb2b5f15c396191f667531a8201172c6b9
---

# 移行時の入力全走査: 計測と方式判断

`checkMigrationSnapshot` の全走査は、cwd なし・平坦な入力1,024会話で初回発見1回と各会話の置換前/commit 前2回、計2,049回だった。今回、ファイル集合の変更と走査失敗を両時点で検出し続けながら全走査回数を会話数から切り離す方式は確認できなかった。製品コードは変更せず、速度改善はない。再計測の生値は [results.json](results.json)、実行手順は [measure.py](measure.py) にある。

## 保証と方式判断

現在の置換前・commit 前の確認は、入力root以下の `.jsonl` 集合を毎回列挙して初回集合と比較し、その会話に属する既知のrollout全bytesも再読込する。追加・削除・rename・新しい下位directory・走査失敗は、その時点の全入力namespaceを観測しなければ取りこぼす。走査しない部分に変更がある状態とない状態は、既知の会話ファイルと観測済みdirectoryだけでは区別できない。確認時点間も含むfilesystem全体の原子的snapshotは従来から保証していない。

検討した方式と判断は次のとおり。

| 方式 | 判断 |
| --- | --- |
| 起動時だけファイル集合を列挙し、各会話では対象rolloutだけを再読込 | 途中の追加・削除・rename・走査失敗をcommit前に検出できないため不採用。 |
| directoryのmtime/ctime、ファイルのsize/mtimeが同じなら全走査を省略 | timestampの復元・粒度・filesystem間の振る舞い、権限変更や新しい下位directoryの走査失敗を含む正確な集合・内容の証明にならないため不採用。 |
| 非同期filesystem watcherの通知がなければ全走査を省略 | 通知の配送時点・取りこぼし・新しい下位directoryへの監視登録とcheck境界の同期を保証できず、通知なしを不変の証明にできないため不採用。 |
| 既知pathの集合と照合するstreaming walkでcopy/sortを省く | 正確な走査のallocationは減らせる可能性があるが、各会話で全入力を2回走査するまま。今回の全走査回数削減は満たさず、別の試行としても実装しなかった。 |

同期的で取りこぼしのない入力namespace変更証拠を提供する別の環境・APIがあれば、全走査の失効条件を再評価できる。現在のGo `filepath.WalkDir` / `os.Stat` とこの移行処理にはその証拠がない。保証を弱めた高速化値を採用版との比較にしないため、候補版とのA/B測定はない。

## 再計測条件と結果

基準は上記commitのclean sourceを `.tmp/snapshot-scan-performance/baseline-source` に抽出したbinary。変更前の `go test -count=1 ./...` はexit 0（3.71秒）。macOS 27.0.1 / 26A434、Go 1.27.1 darwin/arm64、実行前の空き約43 GiB。専用作業領域は156 MiBだった。既存の合成harnessを使い、cwdなしの256/1,024会話、各1 rollout、平坦なinput directoryを測った。入力量はそれぞれ0.678/0.725 MiB、1,024発言ずつ。最適化や計測器を含まない通常buildで未存在DBへの初回と同一snapshot再実行を3組、計測器付きで各1組実行した。表の時間・CPU・RSSは通常buildの中央値と範囲で、走査時間と累積Go allocationは計測器付き1回の観測である。

| 会話/ファイル | 初回全体 秒 | 再実行全体 秒 | 初回CPU 秒 | 初回RSS MiB | 走査回数 | 延べ返却path数 | 初回走査 秒 | 初回allocation MiB |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 256/256 | 0.59 [0.48–0.64] | 0.46 [0.46–0.64] | 0.49 [0.47–0.51] | 36.66 [36.31–37.19] | 513 | 131,328 | 0.187 | 76.01 |
| 1,024/1,024 | 2.61 [2.39–4.02] | 2.80 [2.50–4.30] | 2.68 [2.50–3.54] | 41.88 [41.61–42.13] | 2,049 | 2,098,176 | 1.735 | 757.89 |

CPUはuser+sys、RSSはCLI processのpeak、allocationは起動から終了までの `runtime.MemStats.TotalAlloc` でpeak保持量ではない。計測器付き再実行では走査0.107/1.261秒、allocation 75.07/754.16 MiB。計測器付き初回の置換区間は0.758/3.912秒、CLI全体は1.11/4.02秒だった。走査は置換区間に含まれ、時間を足し合わせない。1,024条件の通常buildも2.39–4.02秒とばらついたため、小差の採否には使えない。以前の[基準計測](../migration-performance-20261008/README.md)の1,024条件は全走査2,049回、走査約1.05秒/全体2.18秒で、今回の回数は一致するが時間は同条件での今回の反復範囲として記録する。

各操作60秒、CLI RSS 1 GiB、専用領域2 GiBを上限にし、250 msごとに自分のprocess groupを監視した。sandbox内の最初の試行は `pgrep` がprocess一覧を取得できず中断したため、その入力を除去し、process監視が使える実行環境で16操作を完了した。中断試行の時間を上表に含めない。全16操作は終了0で、group成功数、初回copyと再実行、旧行削除・他source保持、保存会話/発言/本文長・role・membership・timestamp・cursor終端、SQLite整合性、移行元digestとsidecar不在をharnessが確認した。

再実行時は他のベンチマーク・テストを並行させず、自分のprocessが終了したことを確認してから、この専用 `.tmp/snapshot-scan-performance` だけを除去する。基準commitを抽出し、リポジトリrootで実行する。macOSの `/usr/bin/time -l`、`pgrep`、`ps` を使える環境が必要。

```sh
mkdir -p .tmp/snapshot-scan-performance/baseline-source
git archive 3b0107eb2b5f15c396191f667531a8201172c6b9 go.mod go.sum internal cmd | tar -x -C .tmp/snapshot-scan-performance/baseline-source
python3 cache/snapshot-scan-performance-20261009/measure.py
```

## 保存結果と次タスクへの引き継ぎ

`go test -count=1 ./...` は移行fixtureの初回copy・同一snapshot再実行・拒否・行単位置換、変更・失敗時保持、body/contextを含む欠落rolloutを通した。さらに `multiple_rollouts` fixtureを一時pathへ配置し、CLIを直接4回実行した。初回copyは旧行2件を削除して終了0、同一snapshot再実行はcopyなしで終了0、保存済みrolloutを外した再実行は終了1・group失敗1で本文/旧行/cursorの全行が不変、復元後は再び終了0だった。全段階でsource bytesのdigest・sidecar不在と移行先の `PRAGMA integrity_check=ok` を確認した。CLIのstderrは欠落pathと理由を示し、本文を含まなかった。

製品Goコードの変更はない。最終gateは次のとおりすべてexit 0。所要時間は各コマンドの観測値。

| command | 所要秒 |
| --- | ---: |
| `goimports -l`（対象2ファイル） | <0.01 |
| 全Goの `gofmt -l` 確認 | 0.51 |
| `go test -count=1 ./...` | 3.25 |
| `go vet ./...` | 0.07 |
| `go build -o bin/somniloq ./cmd/somniloq` | 0.25 |
| `go install github.com/google/go-licenses/v2@v2.0.1` | 0.67 |
| `python3 scripts/update-third-party-notices.py --check` | 2.64 |

installはsandbox内のDNS制限で初回失敗し、利用できるnetwork環境で再実行して成功した。notices checkはsandbox外のGo module stat cacheへの書き込み警告を出したがexit 0。

次の本文メモリ保持タスクは今回の全走査方式を変更しない前提で進める。`migrationInput.Files` の入力全体のpath集合は一回の `Migrate` の間だけ保持する。`Group.Reports` のbytesと解析済み本文も現在は全group分を同じ期間保持するが、これを入力総量から切り離すのが次タスクである。確認時のtarget rollout再読込と保存済みbody/contextの欠落検出、旧行・cursorの失敗時保持は継続する。入れ子のdirectory、他filesystem、大規模な単一会話、実ログ、同時変更下の性能は今回測っていない。
