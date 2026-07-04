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
  - internal/core/db_query.go
---

# Configuration and projects

`repo_path` / `--project` / config 周りを変えるときの地図。表示名、filter、集約キー、sessions skip hint 用の command pattern、論理日境界が混ざりやすいので、入口を分けて見る。

## repo_path

- 解決は `internal/core/repo_path.go` の `ResolveRepoPath`。空 cwd は空、git root が取れない cwd は cwd 自体へ fallback。
- import 時は adapter が `RepoResolver` を受け、`SessionMeta.RepoPath` に保存する。
- legacy 補正は `internal/core/backfill.go` の `Backfill`。

## project filter と alias

- config 読み込みは `cmd/somniloq/config.go`。missing file は空 config、invalid JSON は error。
- alias 展開は `config.expandProject`。完全一致したときだけ canonical + old names に展開する。
- `cmd/somniloq/filter.go` の `buildSessionFilter` が time flag と project alias をまとめて `core.SessionFilter` にする。`sessions` / `search` は date-only filter に `dayBoundary` を渡し、`show` / `projects` は従来どおり 00:00 境界で呼ぶ。
- SQL 条件は `internal/core/db_query.go` の `projectsCondition`。repo_path substring LIKE を OR でつなぐ。

## commandPatterns

- `commandPatterns` は `cmd/somniloq/config.go` で読み、invalid regexp は config error にする。壊れた JSON / typo を黙って無効化しない方針に揃える。
- 評価は `commandMatcher`。trim 済み user message 本文が `/` 始まり、または regexp に一致したら command 扱い。
- 利用箇所は `cmd/somniloq/sessions.go` の skip hint 列だけ。セッション自体は CLI では除外しない。

## dayBoundary

- `dayBoundary` は `cmd/somniloq/config.go` の任意設定。形式は `HH:MM`、未指定は `00:00`。invalid value は config error にする。
- `--day-boundary` は `sessions` / `search` だけにあり、config の `dayBoundary` を上書きする。
- `cmd/somniloq/filter.go` の `resolveTimeFlag` は date-only (`YYYY-MM-DD`) のときだけ境界を足す。相対 duration と `YYYY-MM-DDThh:mm` は影響を受けない。
- `sessions` の `logical_day` / `logicalDay` は `sessionLogicalDay` で `ended_at` 優先、無ければ `started_at` を使い、ローカル時刻から境界分を引いた `YYYY-MM-DD`。DB には保存しない。

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
