# Backlog

## v0.4

順序は上から。各タスクは前のタスクが終わってから着手する前提。

- [x] `rules/mission.md` と `rules/scope.md` を Claude Code / Codex 両対応に書き換え（非目標から「Claude Code 以外」を除去、scope を新方針に合わせる）
- [x] DB スキーマ設計確定: `sessions` を `(source, session_id)` 複合主キー化、`messages` に `source` 追加 + 複合外部キー、`import_state` は `jsonl_path` 単独主キー + `source` 補助カラム（詳細は `decisions/0004-codex-schema-and-migration.md`）
- [ ] migration 実装: `backfill` コマンドに v0.3 → v0.4 スキーマ移行（source カラム追加・既存行埋め込み・テーブル再作成による主キー張り替え）を追加。`OpenDB` 内では migration を行わない
- [ ] 共通スキーマ型と adapter インターフェース定義（`internal/ingest/` に `Adapter` interface、`NormalizedRecord` 型などを切る）
- [ ] 既存 Claude Code ingest を adapter 化（現 `internal/core/import.go` の `processFile` を `internal/ingest/claudecode/` に移し、Adapter として再構成。挙動は変えない）
- [ ] Codex adapter 実装（`internal/ingest/codex/`、`response_item` + `payload.type == "message"` + `role in ("user","assistant")` のみ。`session_meta.payload.cwd` から repo_path 解決、`(rollout_path, line_number)` ベースの一意性）
- [ ] CLI: `somniloq import-codex` サブコマンド追加（`somniloq import` は Claude Code 用のまま）。Codex のデフォルトパスは `~/.codex/sessions/`
- [ ] ドキュメント更新（README、scope.md、新 ADR で Codex 対応方針と複合主キー設計を記録）
