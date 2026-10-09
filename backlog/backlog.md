# Backlog

## タスク

### v0.14.2

- [ ] **import のコミット失敗後の transaction 残留と、初発エラー・途中集計の欠落を修正する。** Codex の差分 import で、Commit が失敗すると SQLite 側に transaction が残り、次の Begin が `cannot start a transaction within a transaction` で失敗する経路を合成入力で再現した。後続の Begin 失敗が初発エラーと集計を捨てるため、実行時は stderr にこの1行だけが出て stdout は空だった。実環境の初発 Commit 失敗理由は未確定。
  - 入口は `internal/core/import_codex.go` の Commit / Rollback と Begin 失敗時の返却、`internal/core/db.go` の接続・transaction 管理。Go の `sql.Tx` は Commit 呼び出し後に終了扱いとなるため、Commit エラー後の `tx.Rollback()` だけでは SQLite の残留 transaction を解消できない。共有する transaction 経路への影響を確認し、失敗した接続を残留状態のまま再利用しないようにする。
  - 初発エラーとそれまでの処理結果を保持し、失敗終了時も成功済み件数・失敗理由を CLI から確認できるようにする。失敗した会話の既存本文・関係・cursor と、先に確定した独立会話を保持する。
  - 新規の合成 DB と複数会話の JSONL で、別接続の read transaction による Commit の `SQLITE_BUSY` を再現する。残留 transaction による後続エラーを防ぐこと、初発診断と途中集計、未確定変更の非保存、既存データ保持、競合解消後の再実行を検証し、CLI の stdout・stderr・終了コードと共通 gate を確認する。実 DB は再現 fixture に使わない。

- [ ] **migrate で安全に保持・スキップできた履歴を処理失敗と区別し、警告付きで正常終了できるようにする。** 読み取り専用調査で、`019df6b0-fa00-7f30-8d9a-756170ef326d` は7発言すべてが継承境界 ordinal 82 未満（3〜79）で、parser の本人本文0件という判定は妥当、旧 DB にも同会話・発言はなかった。`019f2c4c-fa7e-7cb2-a965-13c80664b91c` は44発言が旧値と時刻・role・本文ハッシュで一対一に一致したが、旧 UUID は旧 sessions path と行番号でのみ再現でき、現在の archived_sessions path では一致せず、33件で行番号も異なった。元ファイルは残っておらず、移動・編集等の操作は未特定。両 rollout の調査前後の hash は不変だった。現版は前者を `no_own_messages`、後者を `old_input_membership_unknown` としてコマンド全体を失敗扱いにする。
  - 入口は `internal/core/migrate.go` の本人本文判定・旧行所属の判定と、`cmd/somniloq/migrate.go` の summary・診断・終了判定。本人本文がなく、置換対象の旧行や以前に保存した本人本文・cursor を損なわないことを確認できる会話は、対象なしとしてスキップする。既存本文がある会話の本文欠落・欠損を同じ扱いにしない。
  - 新しい本人会話の保存が成功し、入力間の所属証拠の競合はないが、残存ログで旧行の入力所属を証明できない場合は、旧行を削除・吸収せず legacy として保持し、警告として報告する。本文・時刻・role の一致だけで所属証拠を代替しない。正常なコピー・保存・保持が完了し、残るのが安全なスキップと保持の警告だけなら終了0とする。
  - summary と stderr でスキップ・警告・処理失敗を区別し、旧履歴の REF による参照と同一 snapshot の再実行を維持する。ログ集合・内容の変化、解析・所属証拠の競合、I/O・SQL・保存の失敗、以前の本文・cursor を保持すべき不完全入力は引き続き失敗として報告する。
  - 上記2条件を合成 fixture にし、CLI の stdout・stderr・終了コード、旧行保持、新本人会話の保存、失敗時保持、再実行を検証する。`docs/migration.md`・README 両言語・help・移行 fixture の期待値を変更した完了判定に同期し、共通 gate を確認する。稼働中 Codex ログからの移行対応と import の transaction 修正は別 scope とする。
