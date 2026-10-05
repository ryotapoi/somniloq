---
regen: none
sources:
  - internal/core/db.go
  - internal/core/db_schema.go
  - internal/core/db_sessions_projects.go
---

# SQLite driver notes

modernc.org/sqlite と SQLite 固有の外部知見。設計判断や CLI 仕様を拘束し始めたら docs/decisions/ または docs/specs/ へ昇格する。

## modernc.org/sqlite

- `:memory:` は物理接続ごとに別 DB になる。`internal/core/db.go` の `OpenDB` は `SetMaxOpenConns(1)` で 1 接続に固定している。schema 検証用に `sql.Open("sqlite", ":memory:")` を直接使うテストも同じ固定が必要。
- `RowsAffected()` / `LastInsertId()` は modernc.org/sqlite では nil error を返す。

## SQLite

- `INSERT ... ON CONFLICT DO NOTHING` で挿入がスキップされた場合、`LastInsertId()` は今回の文の結果ではなく接続の以前の `last_insert_rowid` を返す。挿入された行の ID として使う前に `RowsAffected()` で実際に挿入されたことを確認する。
- TEXT のバイト数は `OCTET_LENGTH(text)` で取る。`LENGTH(text)` は文字数を返す。`internal/core/db_sessions_projects.go` の `sessionRowSelect` は本文の byte size を `OCTET_LENGTH(m.content)` で集計する。
