---
regen: compiled
sources:
  - docs/rules/scope.md
  - docs/decisions/0011-outline-subcommand-turn-numbering.md
  - docs/decisions/0012-json-output-schema.md
  - docs/decisions/0013-search-time-filter-on-message-timestamp.md
  - cmd/somniloq/show.go
  - cmd/somniloq/format.go
  - cmd/somniloq/outline.go
  - cmd/somniloq/turn.go
  - cmd/somniloq/search.go
  - cmd/somniloq/jsonout.go
  - internal/core/db.go
---

# Display and turns

表示系を変えるときは、cmd の出力整形だけでなく core query の order / sidechain 除外 / JSON shape を揃える。

## ターン採番

- 採番の実装は `cmd/somniloq/turn.go` の `assignTurns`。sidechain を除いた `GetMessages` 全体を渡す前提。
- `show --turn`, `show --tail`, `outline` は同じ `assignTurns` 契約に乗る。片方だけの採番変更は避ける。
- `internal/core/db.go` の `GetMessages` は `timestamp ASC, rowid ASC`。旧 Codex record の timestamp tie を壊すと turn number が揺れる。
- 設計判断は `docs/decisions/0011-outline-subcommand-turn-numbering.md`。

## 表示 path

- Markdown show: `cmd/somniloq/show.go` -> `cmd/somniloq/format.go` -> `internal/core/db.go` の session/message query。
- Summary show: `show.go` が `GetSummaryMessages` に差し替える。`/clear` / `<local-command-caveat>` skip は core query 側。
- Outline: `outline.go` が `GetMessages` と `assignTurns` を使い、user message だけ出す。
- Search: `search.go` が `SearchMessages` の結果に `searchSnippet` をかける。検索の time filter は message timestamp 基準。
- JSON: `cmd/somniloq/jsonout.go`。単一 show も配列で返す。判断は `docs/decisions/0012-json-output-schema.md`。

## 変更時のテスト入口

- `cmd/somniloq/show_turn_test.go`, `cmd/somniloq/turn_test.go`
- `cmd/somniloq/outline_test.go`
- `cmd/somniloq/search_test.go`, `internal/core/db_search_test.go`
- `cmd/somniloq/jsonout_test.go`
