---
regen: compiled
sources:
  - docs/rules/scope.md
  - docs/decisions/0014-project-alias-config.md
  - cmd/somniloq/config.go
  - cmd/somniloq/filter.go
  - cmd/somniloq/shorten.go
  - cmd/somniloq/projects.go
  - cmd/somniloq/search.go
  - internal/core/import.go
  - internal/core/backfill.go
  - internal/core/repo_path.go
  - internal/core/db.go
---

# Configuration and projects

`repo_path` と `--project` 周りを変えるときの地図。表示名、filter、集約キーが混ざりやすいので、入口を分けて見る。

## repo_path

- 解決は `internal/core/repo_path.go` の `ResolveRepoPath`。空 cwd は空、git root が取れない cwd は cwd 自体へ fallback。
- import 時は adapter が `RepoResolver` を受け、`SessionMeta.RepoPath` に保存する。
- legacy 補正は `internal/core/backfill.go` の `Backfill`。

## project filter と alias

- config 読み込みは `cmd/somniloq/config.go`。missing file は空 config、invalid JSON は error。
- alias 展開は `config.expandProject`。完全一致したときだけ canonical + old names に展開する。
- `cmd/somniloq/filter.go` の `buildSessionFilter` が time flag と project alias をまとめて `core.SessionFilter` にする。
- SQL 条件は `internal/core/db.go` の `projectsCondition`。repo_path substring LIKE を OR でつなぐ。

## 集約と表示

- `sessions`, `show`, `search` は `--project` filter の対象。
- `internal/core.DB.ListProjects` は raw `repo_path` ごとの行を返す。`--project` filter は受けず、DB の保存事実は書き換えない。
- 表示名は `cmd/somniloq/shorten.go` の `resolveProjectDisplayName`。alias の canonical / old names が `repo_path` 全体または basename に一致したら canonical 名のみを出す。
- alias 非一致時だけ、`--short` は従来どおり `resolveDisplayName` で basename にする。
- `projects` は `cmd/somniloq/projects.go` で表示名ごとに session count を合算する。alias で同じ canonical 名になる raw `repo_path` 行を重複表示しない。
- `search` は `internal/core.SearchRow.RepoPath` を `cmd/somniloq/search.go` で表示名に変換し、TSV の `project` 列に出す。

## 変更時のテスト入口

- config と alias: `cmd/somniloq/config_test.go`
- time/project filter: `cmd/somniloq/resolve_test.go`, `internal/core/db_query_test.go`, `internal/core/db_search_test.go`
- repo path: `internal/core/repo_path_test.go`
