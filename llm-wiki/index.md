---
regen: full
sources:
  - docs/rules/information-management.md
  - docs/rules/architecture.md
  - docs/rules/scope.md
  - docs/specs/jsonl-schema.md
  - llm-wiki/command-map.md
  - llm-wiki/configuration-and-projects.md
  - llm-wiki/display-and-turns.md
  - llm-wiki/import-pipeline.md
  - llm-wiki/sqlite-driver-notes.md
  - llm-wiki/storage-query-map.md
---

# llm-wiki

変更する領域に応じて、下表から実装・テスト・仕様への経路を選ぶ。複数領域にまたがる変更では、各ページの入口を併せて確認する。

この地図の位置づけ、不変条件、再編纂・配置のルールは `docs/rules/information-management.md` の「llm-wiki/」節を参照する。

| ページ | regen | 内容 | 主なソース |
|---|---|---|---|
| [Command map](command-map.md) | full | CLI コマンドから入口関数・core クエリ・代表テストへ行く索引 | cmd/somniloq, internal/core |
| [Import pipeline](import-pipeline.md) | compiled | Claude Code / Codex / Cursor Agent JSONL が DB 行になるまでの読む順序 | internal/core/import.go, internal/ingest |
| [Storage and query map](storage-query-map.md) | compiled | schema 管理、書き込み、query helper の変更入口 | internal/core/db*.go |
| [Display and turns](display-and-turns.md) | compiled | show の発言選択・ページ・TSV/JSON と現行 search の旧 turn 採番の導線 | cmd/somniloq, internal/core/db_sessions_projects.go, internal/core/db_messages_summary.go, internal/core/db_search.go |
| [Configuration and projects](configuration-and-projects.md) | compiled | repo_path 解決、project alias、project filter の波及先 | cmd/somniloq/config.go, internal/core/repo_path.go |
| [SQLite driver notes](sqlite-driver-notes.md) | none | modernc.org/sqlite / SQLite の外部由来の罠 | internal/core/db.go, internal/core/db_schema.go, internal/core/db_sessions_projects.go |
