# Backlog

## 運用ルール

- Backlog には、今後着手でき、完了を判定できる作業を `- [ ]` 形式のタスクとして置く。
- 対応要否を決める調査もタスクにできる。観察事実だけを残さず、判断したい問いと、判断をもって完了とすることを明示する。
- 粗いメモで起票してよいが、着手前に目的と受け入れ条件を読める粒度へ詰める。
- タスクの行には短い作業名を書き、説明は空行を挟んだ字下げ段落に分ける。
- バージョン番号を付ける場合は SemVer に従う。
- タスクは `###` 見出しでグループに分ける。バージョンが確定したグループだけ、`### v0.11.0 名前` のように見出しへ番号を付ける。
- グループが大きい場合は、`####` 見出しでさらに分割してよい。

## タスク

### v1.0.0 公開機能の整理

- [x] 公開の backfill コマンドを廃止する

  公開の `backfill` コマンドと専用の旧データ修正・移行コードを削除し、関連する help・仕様・README・テストを更新する。利用者は実質プロジェクト所有者のみであるため削除を確定し、旧 DB 利用者向けの専用アップグレード手段は追加しない。一般的な schema 管理は維持する。

- [x] user message の除外を統一する

  `excludeUserMessagePatterns` に統一し、`outline` と `show --summary` に trim 済みの全文へ同じ Go 正規表現を適用する共有 matcher を導入する。config のパターン一覧が未設定なら除外なしとし、繰り返し指定できる CLI pattern は OR で評価して config のパターン一覧全体を置き換える。全除外をその呼び出しだけ無効化する指定を設け、override との併用と不正な regex はエラーにする。空正規表現は無効化の意味にしない。

  除外は first line・件数制限より先に適用し、`outline` は元の turn 番号を維持、summary は除外後の先頭 N 件を表示する。保存 DB、全文 `show` / `--turn` / `--tail`、`search` には適用しない。user message の除外を理由に `sessions` の一覧行を除外しない。

  summary 固定の `/clear`・caveat 除外と `--include-clear` を廃止し、新しい除外指定への移行例を示す。`sessions` 用の `commandPatterns`、slash-prefix 判定、`nonCommandUserTurnCount`、`firstNonCommandUserLine` とその算出処理も廃止し、TSV・JSON の公開出力から削除する。`sessions` に独自の除外ルールは残さず、user message の除外処理は共有 matcher を使う。

  公開出力・設定の変更は v1.0.0 で行い、dayBoundary・logicalDay と summary 自体は維持する。関連する docs・help・README・config/output の移行案内とテストを更新し、設定と CLI の優先順位、共有 matcher と順序、元の turn ID、不正 regex、廃止項目が TSV・JSON に残らないことを自動検証する。

#### 不具合修正

- [ ] 並行 import による本文の欠落を防ぐ

  差分 `import` と `import --full` の並行実行で、成功扱いのまま既存本文が欠落する問題を防ぐ。`internal/core/import.go` には transaction 外で取得した古い offset を全削除後の DB に適用できる経路がある。静的な実行順序で確認した候補なので、2プロセスの順序を制御して再現し、両実行後と次回の差分 import 後に元ログの本文が欠落せず、重複も生じないことを検証する（`I01-001`）。

- [ ] 新規 DB を所有者限定の権限で作成する

  会話ログを保存する新規 DB と新規保存先ディレクトリを、所有者限定の権限で作成する。`cmd/somniloq/main.go` の `openDB` と `internal/core/db.go` の `OpenDB` は、umask 022 の共有パスで DB を 0644 にする。親パスを他ユーザーが辿れる場合にも、新規 DB の本文を他ユーザーが読めないことを確認する。既存 DB の権限変更はこのタスクに含めない（`S01-002`）。

- [ ] repository 解決を Git 環境変数から隔離する

  `internal/core/repo_path.go` の subprocess が呼び出し元の `GIT_DIR`・`GIT_WORK_TREE` 等を継承し、ログの cwd と無関係な root を `repo_path` として保存する問題を防ぐ。別 repository を指す環境下で Claude Code・Codex を取り込み、cwd に基づく正しい保存値・projects 集約・project filter を確認する（`I05-001`）。

- [ ] セッション別のメッセージ取得で全履歴の再走査を減らす

  対象セッション数だけ `messages` 全体を繰り返し走査する問題を解消する。`internal/core/db_messages_summary.go` の `GetMessages` / `GetTurnMessages` と schema の接続を見直し、新規・既存 DB で query plan、複数セッションの `search` / `show` の処理時間、同時刻の rowid 順・turn 採番を確認する。合成10万メッセージ・100セッションの summary 表示では約0.95秒、比較用索引ありでは約0.15秒だった。本文検索の LIKE 全走査は変更しない（`S01-001`）。

- [ ] 不正 timestamp による sessions TSV の破損を防ぐ

  `cmd/somniloq/sessions.go` の時刻欄は `formatLocalTime` が返す不正値の生文字列をそのまま出力する。タブ・改行を含む保存値でも8列・1セッション1行を維持し、JSON の生値と時刻フィルタの既存契約を保つことを検証する（`C03-001`）。

- [ ] RFC3339 時刻引数の不正な offset を拒否する

  `internal/core/duration.go` の `ParseTimeRef` は `+09:60` を `+10:00` 相当、`+24:00` も有効値として扱う。`--since` / `--until` / `--imported-since` で不正 offset をエラーにし、正しい `Z`・数値 offset、相対時刻・ローカル日時の解釈を維持することを確認する（`I06-001`）。

- [ ] Markdown CI を診断結果の問題で失敗させる

  `.github/workflows/ci.yml` は `mdhop diagnose` の終了コードだけを使うが、固定された mdhop v0.16.1 は phantom・壊れた anchor があっても JSON 出力成功時に exit 0 を返す。正常な vault は成功し、`basename_conflicts`・`asset_basename_conflicts`・`phantoms`・`anchors` の対象問題があれば job が失敗することを一時 vault で検証する（`O01-001`）。

- [ ] search の案内を受理される引数順に揃える

  `docs/rules/scope.md` の synopsis は query の後に flag を示すが、`somniloq search auth --since 7d` は `too many arguments` で失敗する。CLI help・README 両言語・正本の案内を照合し、掲載した期間 filter 付きの構文をそのまま実行できることを確認する（`O01-003`）。
