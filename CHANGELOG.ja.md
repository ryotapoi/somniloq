# 変更履歴

[English](CHANGELOG.md) | 日本語

## Unreleased

### 変更

- config を読むサブコマンドの flag 宣言を統合し、help の事前判定で使う flag の認識と値消費判定を、実際の `FlagSet` 定義から導くようにした。
- 未使用の `ingest.Adapter.Source` メソッドを削除した。import source identity は引き続き import source の登録と JSONL 処理で決まる。

## v0.8.0 — 2026-07-12

### 追加

- `import` が parse / normalize に失敗した先頭5件までを、`file:line: error` 形式で stderr に表示するようになり、スキーマ変更の原因を追いやすくなった。既存のサマリ件数、差分取り込みの offset、終了コードの意味は変わらない。

### 変更

- import の transaction 生成、source 別 normalize、query filter、migration / backfill、CLI 整形を共通化し、source 横断の解決、時刻境界、出力スキーマ、import / migration 境界の回帰テストを拡充した。

### 修正

- 永続化に失敗した行を、本文を書き込めた行として数えないようにした。

## v0.7.2 — 2026-07-04

### 変更

- source 共通のメッセージ永続化と CLI / import の補助処理を共通化し、両方の import adapter の保存動作を揃えた。

## v0.7.1 — 2026-07-04

### 変更

- core の DB 責務を schema、write、query、connection の層に分割し、backfill の DB アクセスを DB execer 経由に統一した。
- schema の同等性、migration 競合の再確認、backfill で解決できないパス、PRAGMA 復元失敗、縮小されたファイルの import に対する回帰テストを拡充した。
- ADR 0008 にある `core` → `claudecode` の限定的な依存例外を文書化した。

## v0.7.0 — 2026-07-04

### 追加

- `sessions` が、コマンドでない user turn の件数と先頭行をスキップ判断用ヒントとして出すようになった。`commandPatterns` でコマンド扱いする turn を設定できる。
- 論理日を追加した。`dayBoundary` の設定または `--day-boundary` で `sessions` / `search` の date-only フィルタの基準を指定でき、`sessions` に `logical_day` 列が加わった。
- `outline` が各 turn の本文合計サイズ（応答を含む）を出すようになった。
- 検索結果に、`outline` / `show --turn` と共通の turn 番号を追加した。
- project alias に一致する表示を canonical 名に統一し、`projects` では alias 同士の行をまとめるようにした。`--short` は alias 非一致の project だけを短縮する。
- サブコマンドの help に、出力スキーマ、挙動の注意点、例を追加した。

### 修正

- Codex のルート走査失敗をエラーとして報告するようにした。一方、存在しないルートは未使用 source のまま扱い、子孫の走査失敗は非致命のままとした。
- 壊れた config があっても、`--help` と config 不要のコマンドが失敗しないようにした。サブコマンド間の usage error 整形も統一した。

## v0.6.0 — 2026-06-11

### 追加

- 長いセッションを user-message turn 単位で俯瞰する `outline` を追加した。
- セッションの一部だけを読む `show --turn` と `--tail` を追加した。
- `sessions` 出力に `body_size` を追加した。
- `sessions`、`show`、`projects`、`outline` の JSON 出力を追加した。
- セッション横断検索の `search` を追加した。
- リポジトリ名の変更をまたいで `--project` を展開する `projectAliases` 設定を追加した。

## v0.5.0 — 2026-06-11

### 変更

- `import` のサマリに `unparsed lines` 件数を追加した。サマリを解析するスクリプトは新しい項目に対応する必要がある。
- ディレクトリ走査エラーを非致命にした。読めないディレクトリをスキップして他のファイルを取り込み、エラーは stderr に表示し、エラーが1件でもあれば終了コードは1になる。存在しない source ディレクトリは未使用 source として扱う。
- JSONL 取り込み骨格、サブコマンドの終了コード処理、v0.3 → v0.4 migration の事前条件、SQL 層の sidechain フィルタを共通化し、DB テストを関心ごとに整理した。

## v0.4.0 — 2026-05-05

### 追加

- Claude Code ログに加えて Codex rollout JSONL を正式な取り込み source として追加した。
- `somniloq import --source all|claude-code|codex` と、両 source を横断する `sessions` / `projects` 表示を追加した。
- `backfill` が v0.3 データベースの v0.4 スキーマ移行も行うようになった。

### 変更

- セッションキーを `session_id` だけでなく `(source, session_id)` にした。v0.3 のデータベースは v0.4 で import する前に `somniloq backfill` を1回実行する必要がある。
- `import` は Claude Code と Codex の両方をデフォルトで取り込むようになった。`--source` で adapter を絞り、`--full` は source 指定時でもデータベース全体を削除する。

## v0.3.0 — 2026-05-02

### 追加

- `somniloq backfill` を修復処理の入口として追加した。欠けた `repo_path` を解決し、v0.2.x 由来の孤立セッションを削除する。削除前に確認し、`--yes` で省略できる（破壊的な非対話実行では必須）。

### 変更

- `projects` の集約と `--project` フィルタを `repo_path` のみにした。`project_dir` 列を削除し、古い行は `--project` に一致させる前に `somniloq backfill` が必要になった。
- Git 解決に失敗した場合の `ResolveRepoPath` は `cwd` にフォールバックし、Git 管理外で開始したセッションにも安定したキーを与えるようになった。
- `import` はメタデータだけのレコードからセッションを作らず、会話レコード（`user` / `assistant`）がある場合だけ作るようになった。

## v0.2.1 — 2026-04-22

### 変更

- `--summary N` は件数を受け取り、各セッションの先頭 N 件の user message を表示するようになった（未指定または 0 で無効）。v0.1.x の boolean フラグからの破壊的変更。
- `--summary` はデフォルトで `/clear` の echo と `<local-command-caveat>` ブロックをスキップするようになった。残す場合は `--include-clear` を使う。
- `--summary` が session ID 指定と時間範囲指定の両方で動作するようになった。

### 修正

- `show` の usage 表示で、Go の flag 解析順に合わせて `<session-id>` より前にフラグを置くよう修正した。

## v0.1.1 — 2026-04-02

### 追加

- build 情報に基づく `--version` フラグを追加した。

## v0.1.0 — 2026-04-01

### 追加

- Claude Code のセッションログを import / search する somniloq CLI の初回リリース。
- `~/.claude/projects/` の JSONL ファイルを差分取り込みして SQLite に保存する `import`。
- 時刻・プロジェクトで絞り込み、TSV で出力するセッション一覧。
- セッション数を表示するプロジェクト一覧。
- `show` によるセッション内容の Markdown 表示。
- すべての入力・出力でローカルタイムゾーンをサポート。
- Git worktree のセッションを正規化。
- プロジェクト名を短縮する `--short`。
- セッション概要を素早く確認する `--summary`。
