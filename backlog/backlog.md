# Backlog

## v0.4

順序は上から。各タスクは前のタスクが終わってから着手する前提。

- [x] `rules/mission.md` と `rules/scope.md` を Claude Code / Codex 両対応に書き換え（非目標から「Claude Code 以外」を除去、scope を新方針に合わせる）
- [ ] DB スキーマ設計確定: `sessions.source` カラム（`claude_code` / `codex`）、session_id 主キーを `(source, session_id)` 複合化、`messages.session_id` 外部キーの追従、`import_state` の主キー見直し
- [ ] migration 実装: 既存 `sessions` 行を `source = 'claude_code'` で埋める、複合主キー化、既存ユーザーの v0.3 → v0.4 アップグレード経路を確認（`backfill` で済むか別 migration か）
- [ ] 共通スキーマ型と adapter インターフェース定義（`internal/ingest/` に `Adapter` interface、`NormalizedRecord` 型などを切る）
- [ ] 既存 Claude Code ingest を adapter 化（現 `internal/core/import.go` の `processFile` を `internal/ingest/claudecode/` に移し、Adapter として再構成。挙動は変えない）
- [ ] Codex adapter 実装（`internal/ingest/codex/`、`response_item` + `payload.type == "message"` + `role in ("user","assistant")` のみ。`session_meta.payload.cwd` から repo_path 解決、`(rollout_path, line_number)` ベースの一意性）
- [ ] CLI: `somniloq import-codex` サブコマンド追加（`somniloq import` は Claude Code 用のまま）。Codex のデフォルトパスは `~/.codex/sessions/`
- [ ] ドキュメント更新（README、scope.md、新 ADR で Codex 対応方針と複合主キー設計を記録）
