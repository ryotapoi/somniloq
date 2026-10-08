---
observed_at: 2026-10-05
compiled_at: 2026-10-08
compiled_from_commit: a2f63a7
source_record_commit: 05b837fb831986bb85df986762740f46c7082133
---

# まとまり検索の時点付き計測

原文の記録 commit は計測結果を記した履歴であり、計測時に clean だった正確な binary commit は断定できない。再編纂する場合は `git ls-tree -r --name-only a2f63a7` から `v0.14.0-search-measurements.md` の一致名を探し、`git show a2f63a7:<見つけたpath>` を読む。新たに測る場合は一時 DB を作り、対象 root だけを import した後、同条件の CLI を別 process で3回ずつ実行して件数・経過時間・冷温条件を記録する。実本文は保存しない。

## 実データの取得範囲

元の DB は revision 0 だったため変更せず、実ログから別の一時 DB を構築した。計測に実際に含まれたのは `/Users/ryota/.cursor/projects` の Cursor Agent 入力である。

| 規模 | 値 |
|---|---:|
| JSONL | 708 files、22,648,344 bytes |
| 保存済み会話 | 708 |
| 本人本文 | 5,891 |
| 本文 UTF-8 bytes | 10,804,560 |
| DB bytes | 27,394,048 |
| 初回 import | exit 0、1.6268 秒、708 scanned/imported、0 failed/unparsed |

Claude 全入力を加えた大きいコーパスの一時 import は時間を要したため途中で停止した。その未完了コーパスは検索の計測に含めていない。小さい Claude project ディレクトリを root とする試行では Claude 会話が取り込まれなかったため、上の実測を複数 source の結果として扱わない。元 DB と元ログは変更していない。

## 検索時間

同じ DB に新しい CLI process を起動し、各条件を3回実行した。全て `--format json --limit 0` を指定した。空ページにして出力負荷を抑えているが、total は候補本文の全件照合・関係集約・整列後に算出しており、照合前の limit で高速化していない。

| pattern / 条件 | total | 1回目 秒 | 2回目 秒 | 3回目 秒 |
|---|---:|---:|---:|---:|
| regexp `(?s).*`、候補条件なし | 708 | 0.3757 | 0.0726 | 0.0710 |
| regexp `migration`、source=cursor-agent | 213 | 0.0614 | 0.0613 | 0.0652 |
| pattern なし、候補条件なし | 708 | 0.0199 | 0.0205 | 0.0203 |

例のコマンドは `bin/somniloq search --config CORPUS.toml --format json --limit 0 '(?s).*'`。絞った query は `--source cursor-agent migration`、pattern なしでは位置引数を省く。計測用 TOML は一時 DB の path と上記 Cursor root だけを設定する。

測定値は process 起動、DB の検査・read snapshot、metadata 取得、候補本文照合、集約・整列、JSON 出力を含む経過時間で、regexp 単体の CPU 時間ではない。新しい build の最初の検索とその後の検索で時間差があり、filesystem cache の冷温は制御していない。検索件数・規模・条件・時間だけを記録し、実本文は出力例に転載していない。

このコーパスは Cursor の時刻未知の会話が中心で、実データの大規模 Claude/Codex 関係 graph、多数の legacy snapshot、同時 import 下の性能は測っていない。関係・既知日時・同名 input/source/legacy の意味と snapshot 一貫性は小さい SQLite 回帰 fixture で確認した。実測は取得できたコーパスと query の範囲の証拠であり、全環境の性能保証ではない。
