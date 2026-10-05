---
regen: compiled
sources:
  - docs/rules/scope.md
  - docs/decisions/0014-project-alias-config.md
  - cmd/somniloq/config.go
  - cmd/somniloq/config_init.go
  - cmd/somniloq/filter.go
  - cmd/somniloq/show.go
  - cmd/somniloq/shorten.go
  - cmd/somniloq/projects.go
  - cmd/somniloq/search.go
  - internal/core/import.go
  - internal/core/repo_path.go
  - internal/core/db_sessions_projects.go
  - internal/core/search_groups.go
---

# Configuration and projects

`repo_path` / `--project` / config 周りを変えるときの地図。表示名、filter、集約キー、論理日境界が混ざりやすいので、入口を分けて見る。

## repo_path

- 解決は `internal/core/repo_path.go` の `ResolveRepoPath`。Claude marker を優先し、実在する通常 linked worktree は Git の worktree 情報から本体 root に集約する。通常 repository / submodule は自身の root。空 cwd は空、git root が取れない cwd は cwd 自体へ fallback。保存済み worktree path の再構築条件は `docs/rules/scope.md` の repository 解決節を参照。
- import 時は adapter が `RepoResolver` を受け、`SessionMeta.RepoPath` に保存する。

## project filter と alias

設定形式と alias の契約は `docs/rules/scope.md` の「設定ファイル（config）」、filter の対象は各コマンド節を読む。

- 設定生成は `cmd/somniloq/config_init.go`、読み込みと alias 展開は `cmd/somniloq/config.go`。search の `--project` は `cmd/somniloq/search.go` の `searchCandidateFilter` から `SearchCandidates.accepts` へ進む。候補と root 表示の回帰は `internal/core/search_groups_test.go` を確認する。
- alias の表示への波及は下の「集約と表示」を読む。filter の展開と表示名の解決は別の入口を持つ。

## dayBoundary

境界時刻・DST の契約は `docs/rules/scope.md` の「設定ファイル（config）」、検索と表示への適用は「検索（search）」「内容表示（show）」を読む。

- `cmd/somniloq/config.go` の `resolveDayBoundary` / `parseDayBoundary` から search と show に境界を渡す。日時入力は search の `buildSearchFilter` と show の `parseShowTime` を追う。両者は `dayBoundary.onDate` を共有する。projects の `--since` / `--until` は `cmd/somniloq/filter.go` の `resolveTimeFlag` を使い、設定の dayBoundary を適用しない。
- DST を含む境界の検証入口は `cmd/somniloq/resolve_test.go`、search と show の回帰入口は各コマンドのテストを確認する。

## 集約と表示

- `search` は `--project` filter の対象。show は完全 REF と発言 filter で選択する。
- `internal/core.DB.ListProjects` は raw `repo_path` ごとの行を返す。`--project` filter は受けず、DB の保存事実は書き換えない。時刻条件があると NULL / 空 started_at は対象外、条件なしでは空 repo_path も 1 グループとして残る。
- alias の表示判定は `cmd/somniloq/shorten.go` の `config.canonicalProjectName`。canonical / old names が `repo_path` 全体または basename に一致したら canonical 名を出す。
- alias 非一致時だけ、`--short` は従来どおり `resolveDisplayName` で basename にする。
- `projects` は `cmd/somniloq/projects.go` で表示名ごとに session count を合算する。alias で同じ canonical 名になる raw `repo_path` 行を重複表示しない。
- `search` の候補は `searchCandidateFilter` → `SearchCandidates.accepts`。project は basename substring、表示は root の保存値。projects の alias 表示とは別経路。

## 変更時のテスト入口

- config と alias: `cmd/somniloq/config_test.go`
- time/project filter: `cmd/somniloq/resolve_test.go`, `cmd/somniloq/search_test.go`, `internal/core/search_groups_test.go`, `internal/core/db_sessions_projects_test.go`
- repo path: `internal/core/repo_path_test.go`
