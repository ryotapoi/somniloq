# Backlog

## タスク

### v0.14.2

- [x] **import のコミット失敗後の transaction 残留と、初発エラー・途中集計の欠落を修正する。** Codex の差分 import で、Commit が失敗すると SQLite 側に transaction が残り、次の Begin が `cannot start a transaction within a transaction` で失敗する経路を合成入力で再現した。後続の Begin 失敗が初発エラーと集計を捨てるため、実行時は stderr にこの1行だけが出て stdout は空だった。実環境の初発 Commit 失敗理由は未確定。
  - 入口は `internal/core/import_codex.go` の Commit / Rollback と Begin 失敗時の返却、`internal/core/db.go` の接続・transaction 管理。Go の `sql.Tx` は Commit 呼び出し後に終了扱いとなるため、Commit エラー後の `tx.Rollback()` だけでは SQLite の残留 transaction を解消できない。共有する transaction 経路への影響を確認し、失敗した接続を残留状態のまま再利用しないようにする。
  - 初発エラーとそれまでの処理結果を保持し、失敗終了時も成功済み件数・失敗理由を CLI から確認できるようにする。失敗した会話の既存本文・関係・cursor と、先に確定した独立会話を保持する。
  - 新規の合成 DB と複数会話の JSONL で、別接続の read transaction による Commit の `SQLITE_BUSY` を再現する。残留 transaction による後続エラーを防ぐこと、初発診断と途中集計、未確定変更の非保存、既存データ保持、競合解消後の再実行を検証し、CLI の stdout・stderr・終了コードと共通 gate を確認する。実 DB は再現 fixture に使わない。

- [x] **migrate で安全に保持・スキップできた履歴を処理失敗と区別し、警告付きで正常終了できるようにする。** 読み取り専用調査で、`019df6b0-fa00-7f30-8d9a-756170ef326d` は7発言すべてが継承境界 ordinal 82 未満（3〜79）で、parser の本人本文0件という判定は妥当、旧 DB にも同会話・発言はなかった。`019f2c4c-fa7e-7cb2-a965-13c80664b91c` は44発言が旧値と時刻・role・本文ハッシュで一対一に一致したが、旧 UUID は旧 sessions path と行番号でのみ再現でき、現在の archived_sessions path では一致せず、33件で行番号も異なった。元ファイルは残っておらず、移動・編集等の操作は未特定。両 rollout の調査前後の hash は不変だった。現版は前者を `no_own_messages`、後者を `old_input_membership_unknown` としてコマンド全体を失敗扱いにする。
  - 入口は `internal/core/migrate.go` の本人本文判定・旧行所属の判定と、`cmd/somniloq/migrate.go` の summary・診断・終了判定。本人本文がなく、置換対象の旧行や以前に保存した本人本文・cursor を損なわないことを確認できる会話は、対象なしとしてスキップする。既存本文がある会話の本文欠落・欠損を同じ扱いにしない。
  - 新しい本人会話の保存が成功し、入力間の所属証拠の競合はないが、残存ログで旧行の入力所属を証明できない場合は、旧行を削除・吸収せず legacy として保持し、警告として報告する。本文・時刻・role の一致だけで所属証拠を代替しない。正常なコピー・保存・保持が完了し、残るのが安全なスキップと保持の警告だけなら終了0とする。
  - summary と stderr でスキップ・警告・処理失敗を区別し、旧履歴の REF による参照と同一 snapshot の再実行を維持する。ログ集合・内容の変化、解析・所属証拠の競合、I/O・SQL・保存の失敗、以前の本文・cursor を保持すべき不完全入力は引き続き失敗として報告する。
  - 上記2条件を合成 fixture にし、CLI の stdout・stderr・終了コード、旧行保持、新本人会話の保存、失敗時保持、再実行を検証する。`docs/migration.md`・README 両言語・help・移行 fixture の期待値を変更した完了判定に同期し、共通 gate を確認する。稼働中 Codex ログからの移行対応と import の transaction 修正は別 scope とする。

- [x] **migrate で本人本文0件も正しい置換結果として扱い、同じ ID の旧本文を削除する。** 実在するログを正常に解析した結果が0件なら、旧履歴や保存済み本文・cursor があることを理由に `no_own_messages` で失敗させず、その結果で置き換える。旧版が保存した継承文脈や対象外イベントを、本人本文として残さない。
  - 入口は `internal/core/migrate.go` の本人本文判定・置換処理。現在のログにある会話 ID の旧データを新しい解析結果で置き換え、本人本文0件でも置換を確定する。通常の新規 import と同じ本文・文脈の扱いにそろえ、同じ snapshot の再実行でも結果を維持する。
  - 現在のログに対応する会話 ID が見当たらない旧履歴は、置換対象にせずそのまま legacy として残す。この既存動作は変更しない。
  - 合成 fixture で、旧本文がある会話を本人本文0件へ置換したときの旧本文削除、本文・文脈・関係・cursor の整合、CLI の集計・終了0、再実行を検証する。ログに見当たらない旧履歴が保持されることも確認する。実 DB は fixture に使わない。
  - `docs/migration.md`・README 両言語・help・移行 fixture の期待値を同期し、共通 gate を確認する。

- [x] **migrate を現在のログの解析結果による ID 一致の置換に整理し、会話ごとの全体走査をなくす。** 実行ログ `run-HjlUNTFX` のファイル時刻では、17,190会話の migrate が約50分30秒、続く通常 import が約1分19秒だった。現実装は成功会話ごとに入力 root 全体のファイル集合を2回走査・比較しており、計34,380回の全体走査になる。ID 一致の置換に不要な処理を削除し、通常 import 相当の解析・保存と旧履歴の置換に整理する。
  - 入口は `internal/core/migrate.go` の `checkMigrationSnapshot` と group 置換処理。対象ログを読んで得た解析結果で同じ ID の旧データを置き換え、入力 root 全体の集合不変を会話ごとに要求しない。無関係なログの追加・削除で後続会話を一律失敗にする動作も解消する。本人本文0件の置換と、現在のログに ID が見当たらない旧履歴の保持を維持する。
  - 同じ ID の旧本文削除も、会話ごとに旧メッセージ全体を走査しないようにする。現 SQL の `source,session_id` 条件に対して既存 index は `snapshot_sha256,source,session_id` 順で、実 DB の EXPLAIN は `SCAN legacy_messages` だった。条件と index の対応を確認し、ID で対象を絞って置換できるようにする。
  - 変更前後で解析・走査・SQL・保存の所要時間を計測し、会話数・ファイル数・旧履歴件数を増やした入力で反復全体走査がなくなったことと時間短縮を確認する。通常 import の差分スキップと全件置換の処理量の違いを分けて比較し、今回の50分の内訳は未計測であることを踏まえて原因を検証する。専用の合成 fixture と DB を使い、実 DB を変更する再計測は行わない。
  - ID 一致の置換、0件への置換、対象外旧履歴の保持、再実行、保存失敗時の transaction 整合を検証する。変更した動作に合わせてテスト・`docs/migration.md`・関連する current ADR・必要な README/help を同期し、共通 gate を確認する。

- [x] **migrate の初回走査をセッション metadata の所属確認へ縮小し、全文解析を会話ごとに1回にする。** 全体走査を除去した後の実行ログ `run-7xoskUz8` のファイル時刻では、17,231会話の migrate が約31分20秒、続く差分 import が約1分14秒だった。migrate は初回に全文解析して本文を破棄し、会話ごとの再解析と読み取り前・commit 前の追加ハッシュ確認を行う。各工程の実環境での時間内訳は未計測。全件本文を常駐させず、重複する解析・読み取りを減らす。
  - 入口は `internal/core/migrate.go` の `BuildMigrationIndex`・`checkMigrationGroup`・group 置換、`internal/ingest/codex/group.go` の index/body pass。最初の有効な `session_meta.payload.id` でファイルの所属を決め、`source.subagent.thread_spawn.parent_thread_id` 等の必要な metadata を集める。後続の埋込 metadata や本文中の ID を本人 ID と誤認しない。同じセッション ID のファイルをまとめて全文解析・保存し、その会話の本文を解放する。親子ツリー順の本文読み込みや会話ごとの root 全体 grep は不要。
  - metadata の所属確認と保存時の全文検証を分け、初回の全文解析と追加の全内容ハッシュ照合を削減する。再解析時点の正常な内容を置換結果とし、以後の正常な追記と新しい子セッションは後続の成功した通常 import で取り込む。commit 前の内容不変確認は、この後続取り込みを検証できる条件で廃止する。通常 import が変化検出に使う、実際に解析した bytes のハッシュ・取り込み状態の記録は維持する。追記と削除・移動・破損を区別し、後者の回復を後続 import へ無条件に委ねない。
  - 合成 fixture で、metadata 後の追記、A の保存後に B を処理する間の A への追記、新しい子ログの追加を再現し、migrate と後続 import 後の本文・文脈・親子関係・cursor を静止入力の結果と比較する。同じ ID の複数ファイル、埋込 metadata、本文0件への置換、別 ID の旧履歴保持、所属競合・不完全入力・保存/commit 失敗時保持、再実行も検証する。正常な追記を取りこぼしたまま hash 一致で skip しないことを確認する。
  - 変更前後の初回所属確認・全文解析・ハッシュ確認・SQL/保存の時間と、読み取り bytes・回数・ピークメモリを専用の合成 DB/ログで比較する。会話数と本文量を増やし、全件本文/元 bytes の常駐や反復全体走査を導入せず、時間短縮を確認する。参考として2026-10-10の対象 Codex JSONL は17,231ファイル・15,947,654,119 bytesだったが、元ログ全件保持や実 DB を変更する再計測は行わない。通常 import の初回全件保存と差分 skip を分けて比較する。
  - 変更する読み取り時点・後続 import の契約に合わせてテスト・`docs/migration.md`・current ADR・必要な README/help を同期し、共通 gate を確認する。新 CLI フラグ、watch/retry、全件キャッシュ、通常 import の再設計は対象外。

  - 完了比較: [合成計測](../cache/migration-metadata-performance-20261010/README.md)。64MiB本文/16会話で初回中央値0.84→0.61秒、再実行1.04→0.82秒。正常追記は後続 import で静止入力へ収束、移行中の既存 prefix 編集・削除・移動は利用契約で禁止し、stat が検出できない伸長書換等は未保証とした。
