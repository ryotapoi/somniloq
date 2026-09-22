# Backlog

## 運用ルール

- Backlog には、今後着手でき、完了を判定できる作業を `- [ ]` 形式のタスクとして置く。
- 対応要否を決める調査もタスクにできる。観察事実だけを残さず、判断したい問いと、判断をもって完了とすることを明示する。
- 粗いメモで起票してよいが、着手前に目的と受け入れ条件を読める粒度へ詰める。
- バージョン番号を付ける場合は SemVer に従う。
- タスクは `###` 見出しでグループに分ける。バージョンが確定したグループだけ、`### v0.11.0 名前` のように見出しへ番号を付ける。
- グループが大きい場合は、`####` 見出しでさらに分割してよい。

## タスク

### v1.0.0 公開機能の整理

- [ ] 公開の `backfill` コマンドと専用の旧データ修正・移行コードを削除する。実装時は関連する help・仕様・README・テストを更新し、v0.3 から v0.4 への移行が現在は `backfill` 経由でしか行われないため、v1.0.0 より前の版からのアップグレード手順を明確にする。新たな自動移行の導入や一般的な schema 管理の削除は含めない。
- [ ] `outline` と `show --summary` に、trim 済みの user message 全文へ同じ Go 正規表現を適用する除外を導入する。
  - config の `excludeUserMessagePatterns` を未設定なら除外なしとし、繰り返し指定できる CLI pattern は OR で評価して指定時に config のパターン一覧全体を置き換える。全除外をその呼び出しだけ無効化する指定を設け、override と無効化の併用はエラーにする（空正規表現を無効化の意味にしない）。不正な regex もエラーにする。
  - 除外は first line・件数制限より先に適用し、`outline` は元の turn 番号を維持、summary は除外後の先頭 N 件を表示する。保存 DB、全文 `show` / `--turn` / `--tail`、`search`、`sessions` の一覧には適用しない。
  - `commandPatterns`、slash-prefix 判定、固定の `/clear`・caveat 除外、`--include-clear`、`nonCommandUserTurnCount`、`firstNonCommandUserLine` を削除し、既存の clear・caveat・AGENTS 用 pattern は config 例と移行案内へ移す。dayBoundary・logicalDay と summary 自体の廃止は含めない。関連する docs・help・README・config/output の移行案内を更新し、設定と CLI の優先順位、共有 matcher と順序、元の turn ID、不正 regex を自動検証する。
