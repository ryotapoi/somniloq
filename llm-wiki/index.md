---
regen: full
sources:
  - docs/rules/information-management.md
  - docs/rules/architecture.md
  - docs/rules/scope.md
  - docs/specs/jsonl-schema.md
  - llm-wiki/agent-workflow-map.md
  - llm-wiki/command-map.md
  - llm-wiki/configuration-and-projects.md
  - llm-wiki/display-and-turns.md
  - llm-wiki/import-pipeline.md
  - llm-wiki/sqlite-driver-notes.md
  - llm-wiki/storage-query-map.md
---

# llm-wiki

| ページ | regen | 内容 | 主なソース |
|---|---|---|---|
| [Command map](command-map.md) | full | CLI コマンドから入口関数・core クエリ・代表テストへ行く索引 | cmd/somniloq, internal/core |
| [Import pipeline](import-pipeline.md) | compiled | Claude Code / Codex JSONL が DB 行になるまでの読む順序 | internal/core/import.go, internal/ingest |
| [Storage and query map](storage-query-map.md) | compiled | schema, migration, query helper, backfill の変更入口 | internal/core/db.go, internal/core/backfill.go |
| [Display and turns](display-and-turns.md) | compiled | show / outline / search の表示・ターン採番・TSV/JSON の導線 | cmd/somniloq, internal/core/db.go |
| [Configuration and projects](configuration-and-projects.md) | compiled | repo_path 解決、project alias、project filter の波及先 | cmd/somniloq/config.go, internal/core/repo_path.go |
| [Agent workflow map](agent-workflow-map.md) | compiled | Codex / Claude 側 workflow, skill, docs 配置の同期を見る導線 | AGENTS.md, CLAUDE.md, .agents, .claude |
| [SQLite driver notes](sqlite-driver-notes.md) | none | modernc.org/sqlite / SQLite の外部由来の罠 | internal/core/db.go, internal/core/backfill.go |
