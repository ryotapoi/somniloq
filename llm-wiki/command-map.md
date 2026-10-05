---
regen: full
sources:
  - cmd/somniloq/main.go
  - cmd/somniloq/config_init.go
  - cmd/somniloq/import.go
  - cmd/somniloq/migrate.go
  - cmd/somniloq/show.go
  - cmd/somniloq/show_tsv.go
  - cmd/somniloq/search.go
  - cmd/somniloq/search_detail.go
  - cmd/somniloq/filter.go
  - cmd/somniloq/projects.go
  - cmd/somniloq/jsonout.go
  - internal/core/db.go
  - internal/core/db_import_state.go
  - internal/core/db_sessions_projects.go
  - internal/core/db_messages_summary.go
  - internal/core/search_groups.go
  - internal/core/search_matches.go
  - internal/core/import.go
  - internal/core/migrate.go
  - docs/rules/scope.md
  - docs/decisions/0012-json-output-schema.md
---

# Command map

CLI 入口を触る前に、まずこの表で「cmd 層」「core 層」「仕様/テスト」を揃える。cmd 層はフラグ・出力・exit code を持つが、DB 操作と JSONL パースは持たない。

| コマンド | cmd 入口 | core / ingest 側 | 代表テスト | 仕様ポインタ |
|---|---|---|---|---|
| global routing | `cmd/somniloq/main.go` | `internal/core/db.go` の `OpenDB` | `cmd/somniloq/main_dispatch_test.go`, 各 cmd test | `docs/rules/scope.md` の CLI インターフェース |
| `config init` | `cmd/somniloq/config_init.go` | DB を開かない | `cmd/somniloq/config_test.go` | `docs/rules/scope.md` の 設定ファイル |
| `import` | `cmd/somniloq/import.go` | `internal/core/import.go`, `internal/ingest/*` | `cmd/somniloq/import*_test.go`, `internal/core/import_test.go`, `internal/core/codex_import_test.go` | `docs/rules/scope.md` の 取り込み |
| `migrate` | `cmd/somniloq/migrate.go` | `internal/core/migrate.go` | `cmd/somniloq/migrate_test.go`, `internal/core/migrate_fixture_test.go` | `docs/specs/v0.14.0-migration.md` |
| `show` | `cmd/somniloq/show.go`, `cmd/somniloq/show_tsv.go`, `cmd/somniloq/jsonout.go` | `session_relations.go` の `ResolveSession`、`db_messages_summary.go` の `GetIdentityMessages` を同じ `ReadSnapshot` で利用。発言 filter / page / one-line は cmd 層 | `cmd/somniloq/show_contract_test.go`, `cmd/somniloq/show_filters_test.go`, `cmd/somniloq/jsonout_test.go` | `docs/rules/scope.md` の 内容表示 |
| `search` | `cmd/somniloq/search.go`, `cmd/somniloq/search_detail.go` | `SearchGroups` / `SearchOccurrences`（候補本文・matcher・snapshot を共有） | `cmd/somniloq/search_test.go`, `cmd/somniloq/search_detail_test.go`, `internal/core/search_groups_test.go` | `docs/rules/scope.md` の 検索 |
| `projects` | `cmd/somniloq/projects.go` | `internal/core/db_sessions_projects.go` の `ListProjects` | `cmd/somniloq/jsonout_test.go`, `internal/core/db_sessions_projects_test.go` | `docs/rules/scope.md` の プロジェクト一覧 |

## 変更時の読む順序

- 新フラグや出力列を足す: cmd 入口 -> `docs/rules/scope.md` -> README 両方 -> cmd test -> core query test。
- JSON 出力を変える: `cmd/somniloq/jsonout.go` -> 対象 cmd -> `docs/decisions/0012-json-output-schema.md` -> JSON tests。
- time / project filter を変える: `cmd/somniloq/search.go` の `buildSearchFilter` / `searchCandidateFilter` と `cmd/somniloq/filter.go` の projects 用日時 filter -> `internal/core/search_groups.go` の候補判定または `internal/core/db_sessions_projects.go` の `timeFilterConditions` -> query tests。
- コマンド追加: `cmd/somniloq/main.go` の routing と usage、`docs/rules/scope.md`、README 両方、コマンド固有 test を同時に見る。
