# ADR 0005: Codex rollout JSONL の取り込み方針

## Status

Accepted

## Context

v0.4 で Codex の rollout JSONL を取り込む。スキーマと migration 方針は ADR 0004 で決定済みだが、adapter 実装時点で以下の実装方針を確定する必要があった。

1. Claude Code 用 `import` と Codex 用 import を同じサブコマンドにまとめるか
2. Codex のどの JSONL レコードを `messages` に保存するか
3. Codex message に Claude Code の `uuid` 相当が無い前提で `messages.uuid` をどう作るか
4. 差分取り込み時に、ファイル先頭の `session_meta` をどう扱うか

## Considered Options

### 1. CLI サブコマンド

- **`import-codex` を追加**: Claude Code の `import` は従来どおり `~/.claude/projects/` 専用にし、Codex は `~/.codex/sessions/` 専用の入口に分ける。source ごとのデフォルトパスと JSONL 形式の違いが CLI 上も明確になる
- **`import --source codex` を追加**: サブコマンドは増えないが、既存 `import` の意味が広がり、デフォルトパスや help の説明が複雑になる

### 2. 保存対象レコード

- **`response_item` の message role のみ保存**: `payload.type == "message"` かつ `role` が `user` / `assistant` のレコードだけを保存し、tool call、reasoning、event は捨てる。Claude Code 側の「会話 text のみ保存」と揃う
- **`response_item` を広く保存**: function call や output も保存する。後から調査できる情報は増えるが、`messages` テーブルの「会話ターン」モデルから外れ、既存 show / summary の意味が崩れる

### 3. message UUID

- **`(rollout_path, line_number)` から決定的に生成**: Codex rollout 内の同じ行は再実行しても同じ ID になり、差分取り込みと full re-import の重複判定に使える
- **content hash から生成**: 同じ内容の発話が複数回出ると衝突しうる。timestamp を足しても、同時刻や欠損時の扱いが不安定になる

### 4. 差分取り込み時の session_meta

- **offset 前の `session_meta` を読み直す**: 通常 `session_meta` はファイル先頭にあり、追記分だけを読むと cwd / version / branch / session_id を失うため、追記処理前に prefix を走査して復元する
- **初回 import 時の DB 行だけに頼る**: 追記メッセージの session upsert は可能だが、adapter が DB read API を必要とし、`internal/ingest` と `internal/core` の責務境界が太くなる

## Decision

- DB スキーマは ADR 0004 の `(source, session_id)` 複合主キー設計を前提にする。Codex は `source='codex'` と `session_meta.payload.id` の組で Claude Code セッションと分離する
- `somniloq import-codex` を追加し、Codex のデフォルトパスは `~/.codex/sessions/` とする。既存 `somniloq import` は Claude Code 用のまま維持する
- Codex adapter は `internal/ingest/codex/` に置き、`internal/core` や `cmd/somniloq` へ依存しない
- 保存対象は `response_item` かつ `payload.type == "message"` かつ `role in ("user", "assistant")` のみ
- `payload.content` は `input_text` / `output_text` / `text` block の `text` のみ抽出し、複数 block は空行区切りで結合する
- `session_id` は `session_meta.payload.id` を使い、ファイル名 stem は走査用の補助情報に留める
- `messages.uuid` は `(rollout_path, line_number)` を入力にした決定的 ID とする
- 差分取り込みでは、offset 直前までの `session_meta` を先に読み直してから追記分を処理する

## Consequences

- Claude Code と Codex の JSONL 形式差は adapter 配下に閉じ、`internal/core` は source ごとの adapter を選ぶだけで済む
- CLI では `import` と `import-codex` が分かれるため、source ごとのデフォルトパスが明確になる
- function call / reasoning / event は保存されない。somniloq の現行スコープは「会話 text の検索・閲覧」であり、tool 実行履歴の完全保存は非目標として扱う
- rollout ファイルの途中に過去行が挿入されるような形式変更が起きると、line number ベース ID は変わりうる。現行 rollout は追記型ログとして扱い、形式が変わったら `references/jsonl-schema.md` と adapter を見直す

## References

- `rules/scope.md`
- `references/jsonl-schema.md`
- `decisions/0004-codex-schema-and-migration.md`
- `internal/ingest/codex/`
