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

- [x] 公開の `backfill` コマンドと専用の旧データ修正・移行コードを削除し、関連する help・仕様・README・テストを更新する。利用者は実質プロジェクト所有者のみであるため削除を確定し、旧 DB 利用者向けの専用アップグレード手段は追加しない。一般的な schema 管理は維持する。
- [x] user message の除外を `excludeUserMessagePatterns` に統一し、`outline` と `show --summary` に trim 済みの全文へ同じ Go 正規表現を適用する共有 matcher を導入する。
  - config の `excludeUserMessagePatterns` を未設定なら除外なしとし、繰り返し指定できる CLI pattern は OR で評価して指定時に config のパターン一覧全体を置き換える。全除外をその呼び出しだけ無効化する指定を設け、override と無効化の併用はエラーにする（空正規表現を無効化の意味にしない）。不正な regex もエラーにする。
  - 除外は first line・件数制限より先に適用し、`outline` は元の turn 番号を維持、summary は除外後の先頭 N 件を表示する。保存 DB、全文 `show` / `--turn` / `--tail`、`search` には適用しない。user message の除外を理由に `sessions` の一覧行を除外しない。
  - summary 固定の `/clear`・caveat 除外と `--include-clear` は廃止し、新しい除外指定への移行例を示す。`sessions` 用の `commandPatterns`、slash-prefix 判定、`nonCommandUserTurnCount`、`firstNonCommandUserLine` とその算出処理も廃止し、TSV・JSON の公開出力から該当項目を削除する。`sessions` に独自の除外ルールは残さず、user message の除外処理は共有 matcher を使う。
  - 公開出力・設定の変更は v1.0.0 で行う。dayBoundary・logicalDay と summary 自体の廃止は含めない。関連する docs・help・README・config/output の移行案内とテストを更新し、設定と CLI の優先順位、共有 matcher と順序、元の turn ID、不正 regex、廃止項目が TSV・JSON に残らないことを自動検証する。
