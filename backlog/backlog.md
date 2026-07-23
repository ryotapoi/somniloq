# Backlog

## v0.9.0（2026-07-17 maintenance-audit より・エラーメッセージが変わる）

- [ ] DB query error に操作文脈を付ける
  - `internal/core/db_query.go` の public query 境界（`ListSessions` 等）が driver error をそのまま返している。DB 破損・lock・schema 変更時にどの query が失敗したか切り分けにくい
  - 操作名を付けつつ `%w` と既存の `sql.ErrNoRows` 契約を維持する
