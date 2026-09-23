# Backlog

## 運用ルール

- Backlog には、今後着手でき、完了を判定できる作業を `- [ ]` 形式のタスクとして置く。
- 対応要否を決める調査もタスクにできる。観察事実だけを残さず、判断したい問いと、判断をもって完了とすることを明示する。
- 粗いメモで起票してよいが、着手前に目的と受け入れ条件を読める粒度へ詰める。
- バージョン番号を付ける場合は SemVer に従う。
- タスクは `###` 見出しでグループに分ける。バージョンが確定したグループだけ、`### v0.11.0 名前` のように見出しへ番号を付ける。
- グループが大きい場合は、`####` 見出しでさらに分割してよい。

## タスク

### v0.12.1 不具合修正

- [ ] `import --full` の `DeleteAll` で `messages`・`sessions`・`import_state` の削除を既存の transaction 内で不可分にし、削除途中に失敗しても3表がすべて元の状態を保つことを確認する。部分削除のため後続の差分 import で行が復元できなくなる不具合を直し、新しい抽象化は追加しない。
- [ ] `GetMessages`・`GetSummaryMessages`・`SearchMessages`・`ListSessions`・`ListProjects` と `UpsertSession` の `started_at` / `ended_at` 選択で、時刻の順序比較を既存の時点比較機構に揃える。保存文字列と schema は変えず、offset や小数桁が異なる有効な RFC3339 時刻で並び・集約・最初と最後の選択が正しく、空の開始時刻が後続の実時刻で更新され、同時刻の `rowid` 順が保たれることを確認する。
- [ ] import の2つの stderr 診断ループで出力エラーを既存のエラー返却経路から返し、unparsed 診断だけの場合も出力失敗で成功終了しないようにする。失敗する writer で終了結果と返却 error を確認する。
- [ ] `internal/core/import_test.go`（`TestImport_Full` を含む）と `internal/core/codex_import_test.go` で未検査の `QueryRow.Scan` エラーを検査し、SQL 取得に失敗したとき以前の count 値でテストが誤成功しないようにする。既存テストの期待値と production 挙動を維持して core tests が通ることを確認し、新しい汎用 helper は追加しない。
- [ ] `llm-wiki/display-and-turns.md` と `storage-query-map.md` を現行 scope と source から再編纂し、search から show / outline への source 指定と、`sessionRowColumns` を SELECT / scan 変更時の入口として案内する。読む場所・変更箇所の判断に必要な案内だけを残し、仕様の再掲や判断に影響しない説明を削る。実 symbol とリンクの整合を確認する。
- [ ] `docs/rules/scope.md`、`README.md`、`README.ja.md` で `imported_at` を「session を最後に保存更新した JSONL ファイル処理の開始時刻」と明記し、CLI 呼出し全体や最新 invocation の時刻、スキップされたファイルの時刻とは読めないよう曖昧な `import pass` 表現を置き換える。`--imported-since` の判断に必要な意味と既存の UTC 秒精度・non-watermark の説明を保ち、runtime は変更しない。
- [ ] `internal/core/repo_path.go` と対応する `repo_path_test.go` のコメントを、変更判断に影響する API 契約と非自明な理由に限って英語で残す（empty cwd、Git invocation、path whitespace の保持、stderr suppression の理由）。実装と食い違う説明は実装に合わせ、コードの言い換えと将来の logging 手順を削る。動作とテスト期待値は変えず、既存テストで確認する。コメント全体の一律翻訳はしない。
- [ ] VF-021: `import --full` の確認分岐を既存の `importCmd`（reader/writer/`isTTY`/`openDB` 注入済み）で検証し、`--yes` なしの非TTY拒否とTTY拒否はDBを開かず副作用がなく、TTY承認と非TTY `--yes` はfull importへ進むことを、代表的な終了コード・error・出力と既存一時DBでの削除・再取込により確認する。`confirmYesNo` の入力variantやcoreのfull SQLテストを重複させず、production変更・hook・汎用helperは追加しない。
- [ ] VF-022: 既存のCursor fixture/incremental testのassertionだけを拡張し、保存identityがpathとphysical line（ignored/broken/blank行を含む）から決まり、追記後も物理行番号が続くこと、再処理でUUIDが保たれ重複しないこと、unknown timestamp/model等が宣言済みmetadata契約どおり空であることを確認する。別のE2E suiteやhash algorithm helperのunit caseを増やさず、wall clock fallbackやproduction変更を加えない。
- [ ] VF-023: `codex.normalizeMessage` の直接unit testで、timestampのないresponseが `session_meta.Timestamp` を継承することを確認する。timestamp明示時の優先順位は既存テストを再利用し、core/CLI統合テストやproduction変更・hookは追加しない。
- [ ] VF-040: coreの `ImportResult.add` / `addUnparsedDiagnostics` を直接呼ぶテストで、複数batch（複数file相当）をまたいでも診断が遭遇順の先頭5件に保たれ、上限到達後のbatchから増えないことを確認する。既存parserの上限テストと重複するdisk/CLI fixture、production変更・hook・汎用helperは追加しない。
- [ ] VF-041: 既存の `importWithAdapter` 境界とsetupを使い、小さなtest doubleでstat失敗（走査結果にnonexistent pathを返す）と `ProcessFile` errorを別々に発生させ、各々で `FilesFailed` / `Errors` と後続の成功file処理継続を確認する。permission race・sleep・global production hook・新しいDIを導入せず、冗長な全source/CLI matrixを作らない。
- [ ] VF-042: 既存の `internal/core/import_test.go` と `cmd/somniloq/import_test.go` の診断assertionで、`encoding/json` の文言やGo内部型名への全文一致を避け、件数・順序・file:line・詳細非空とCLIのstderr形式・終了結果・summaryを検証する。production変更・新test suite・汎用helperは加えない。

### v1.0.0 公開機能の整理

- [ ] 公開の `backfill` コマンドと専用の旧データ修正・移行コードを削除する。実装時は関連する help・仕様・README・テストを更新し、v0.3 から v0.4 への移行が現在は `backfill` 経由でしか行われないため、v1.0.0 より前の版からのアップグレード手順を明確にする。新たな自動移行の導入や一般的な schema 管理の削除は含めない。
- [ ] `outline` と `show --summary` に、trim 済みの user message 全文へ同じ Go 正規表現を適用する除外を導入する。
  - config の `excludeUserMessagePatterns` を未設定なら除外なしとし、繰り返し指定できる CLI pattern は OR で評価して指定時に config のパターン一覧全体を置き換える。全除外をその呼び出しだけ無効化する指定を設け、override と無効化の併用はエラーにする（空正規表現を無効化の意味にしない）。不正な regex もエラーにする。
  - 除外は first line・件数制限より先に適用し、`outline` は元の turn 番号を維持、summary は除外後の先頭 N 件を表示する。保存 DB、全文 `show` / `--turn` / `--tail`、`search`、`sessions` の一覧には適用しない。
  - `commandPatterns`、slash-prefix 判定、固定の `/clear`・caveat 除外、`--include-clear`、`nonCommandUserTurnCount`、`firstNonCommandUserLine` を削除し、既存の clear・caveat・AGENTS 用 pattern は config 例と移行案内へ移す。dayBoundary・logicalDay と summary 自体の廃止は含めない。関連する docs・help・README・config/output の移行案内を更新し、設定と CLI の優先順位、共有 matcher と順序、元の turn ID、不正 regex を自動検証する。
