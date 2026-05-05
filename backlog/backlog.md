# Backlog

## v0.4

順序は上から。各タスクは前のタスクが終わってから着手する前提。

- [x] `rules/mission.md` と `rules/scope.md` を Claude Code / Codex 両対応に書き換え（非目標から「Claude Code 以外」を除去、scope を新方針に合わせる）
- [x] DB スキーマ設計確定: `sessions` を `(source, session_id)` 複合主キー化、`messages` に `source` 追加 + 複合外部キー、`import_state` は `jsonl_path` 単独主キー + `source` 補助カラム（詳細は `decisions/0004-codex-schema-and-migration.md`）
- [x] migration 実装: `backfill` コマンドに v0.3 → v0.4 スキーマ移行（source カラム追加・既存行埋め込み・テーブル再作成による主キー張り替え）を追加。`OpenDB` 内では migration を行わない
- [x] 共通スキーマ型・adapter interface 定義 + 既存 Claude Code ingest の adapter 化（`internal/ingest/` に `Adapter` interface と `NormalizedRecord` 等の共通型を切り、現 `internal/core/import.go` の `processFile` を `internal/ingest/claudecode/` に移して Adapter として再構成。interface は実装と一緒に磨く前提で 1 タスクに統合。挙動は変えない）
- [x] Codex adapter 実装（`internal/ingest/codex/`、`response_item` + `payload.type == "message"` + `role in ("user","assistant")` のみ。`session_meta.payload.cwd` から repo_path 解決、`(rollout_path, line_number)` ベースの一意性）
- [x] CLI: `somniloq import-codex` サブコマンド追加（`somniloq import` は Claude Code 用のまま）。Codex のデフォルトパスは `~/.codex/sessions/`
- [x] ドキュメント更新（README、scope.md、新 ADR で Codex 対応方針と複合主キー設計を記録）
- [x] CLI: 推奨形へ変更する。`somniloq import` は Claude Code / Codex の両方を同じ SQLite DB へ取り込み、対象を絞る場合は `--source all|claude-code|codex` を使う。リリース前のため `import-codex` は互換 shim として残さず整理する
- [x] examples/skills/somniloq を v0.4 に合わせて更新する。Claude Code 専用の説明を Claude Code / Codex 両対応へ直し、`somniloq import` の両 source 取り込み、`--source`、`~/.claude/projects/` / `~/.codex/sessions/`、v0.4 backfill 前提を反映する
