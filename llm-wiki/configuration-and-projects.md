---
regen: compiled
sources:
  - docs/rules/scope.md
  - docs/decisions/0014-project-alias-config.md
  - cmd/somniloq/config.go
  - cmd/somniloq/filter.go
  - cmd/somniloq/sessions.go
  - cmd/somniloq/show.go
  - cmd/somniloq/turn.go
  - cmd/somniloq/shorten.go
  - cmd/somniloq/projects.go
  - cmd/somniloq/search.go
  - internal/core/import.go
  - internal/core/backfill.go
  - internal/core/repo_path.go
  - internal/core/db_sessions_projects.go
  - internal/core/db_search.go
---

# Configuration and projects

`repo_path` / `--project` / config 周りを変えるときの地図。表示名、filter、集約キー、sessions skip hint 用の command pattern、論理日境界が混ざりやすいので、入口を分けて見る。

## repo_path

- 解決は `internal/core/repo_path.go` の `ResolveRepoPath`。Claude marker を優先し、実在する通常 linked worktree は Git の worktree 情報から本体 root に集約する。通常 repository / submodule は自身の root。空 cwd は空、git root が取れない cwd は cwd 自体へ fallback。保存済み worktree path の再構築条件は `docs/rules/scope.md` の repository 解決節を参照。
- import 時は adapter が `RepoResolver` を受け、`SessionMeta.RepoPath` に保存する。
- legacy 補正は `internal/core/backfill.go` の `Backfill`。

## project filter と alias

設定形式と alias の契約は `docs/rules/scope.md` の「設定ファイル（config）」、filter の対象は各コマンド節を読む。

- 設定の読み込みは `cmd/somniloq/config.go` の `loadConfig`。filter への受け渡しは `cmd/somniloq/filter.go` の `buildSessionFilter` / `buildSessionFilterAt` から `config.expandProject` を辿る。
- 展開後の `core.SessionFilter.Projects` は `internal/core/db_sessions_projects.go` の `sessionFilterConditions` → `projectsCondition` → `escapeLikeLiteral` へ進む。条件を変える際は `ListSessions` と `internal/core/db_search.go` の `SearchMessages` を併せて確認する。
- alias の表示への波及は下の「集約と表示」を読む。filter の展開と表示名の解決は別の入口を持つ。

## commandPatterns

判定規則は `docs/rules/scope.md` の「設定ファイル（config）」、出力列の契約は「セッション一覧（sessions）」を参照する。

- `cmd/somniloq/config.go` の `loadConfig` / `newCommandMatcher` が `compileCommandPatterns` を使い、`commandMatcher.isCommand` が本文を判定する。
- 利用側は `cmd/somniloq/sessions.go` の `deriveSessionUserTurnSummaries` → `summarizeNonCommandUserTurns`。判定対象のターンを変える場合は `cmd/somniloq/turn.go` の `assignTurns` / `userTurnMessages` と [Display and turns](display-and-turns.md) を併せて確認する。

## dayBoundary

境界時刻・DST の契約は `docs/rules/scope.md` の「設定ファイル（config）」、filter と表示への適用は「セッション一覧（sessions）」と「検索（search）」を読む。

- `cmd/somniloq/config.go` の `resolveDayBoundary` / `parseDayBoundary` から、sessions / search の filter 構築へ値が渡る。呼び出し側を変える際は、`show.go` / `projects.go` が渡す `dayBoundary{}` との違いも確認する。
- 日付 filter は `cmd/somniloq/filter.go` の `resolveTimeFlag`、論理日表示は同ファイルの `sessionLogicalDay` が入口。両者はローカル暦日の境界を作る `dayBoundary.onDate` を共有するため、境界の変更は両経路を併せて確認する。
- DST を含む境界の検証入口は `cmd/somniloq/resolve_test.go`。表示への受け渡しは `cmd/somniloq/sessions.go` の TSV / JSON 両経路を読む。

## 集約と表示

- `sessions`, `show`, `search` は `--project` filter の対象。
- `internal/core.DB.ListProjects` は raw `repo_path` ごとの行を返す。`--project` filter は受けず、DB の保存事実は書き換えない。時刻条件があると NULL / 空 started_at は対象外、条件なしでは空 repo_path も 1 グループとして残る。
- 表示名は `cmd/somniloq/shorten.go` の `resolveProjectDisplayName`。alias の canonical / old names が `repo_path` 全体または basename に一致したら canonical 名のみを出す。
- alias 非一致時だけ、`--short` は従来どおり `resolveDisplayName` で basename にする。
- `projects` は `cmd/somniloq/projects.go` で表示名ごとに session count を合算する。alias で同じ canonical 名になる raw `repo_path` 行を重複表示しない。
- `search` は `internal/core.SearchRow.RepoPath` を `cmd/somniloq/search.go` で表示名に変換し、TSV の `project` 列に出す。

## 変更時のテスト入口

- config と alias: `cmd/somniloq/config_test.go`
- time/project filter: `cmd/somniloq/resolve_test.go`, `internal/core/db_sessions_projects_test.go`, `internal/core/db_search_test.go`
- repo path: `internal/core/repo_path_test.go`
