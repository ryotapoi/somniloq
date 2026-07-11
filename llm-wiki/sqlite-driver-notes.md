---
regen: none
sources:
  - internal/core/db.go
  - internal/core/db_schema.go
  - internal/core/db_query.go
  - internal/core/migrate_v04.go
  - internal/core/backfill.go
  - internal/core/backfill_test.go
---

# SQLite driver notes

modernc.org/sqlite と SQLite 固有の外部知見。設計判断や CLI 仕様を拘束し始めたら docs/decisions/ または docs/specs/ へ昇格する。

## modernc.org/sqlite

- `:memory:` は物理接続ごとに別 DB になる。`internal/core/db.go` の `OpenDB` は `SetMaxOpenConns(1)` で 1 接続に固定している。migration 用に `sql.Open("sqlite", ":memory:")` を直接使うテストも同じ固定が必要。
- `RowsAffected()` / `LastInsertId()` は modernc.org/sqlite では nil error を返す。`internal/core/backfill.go` の `RowsAffected()` error check は、現ドライバのためではなく将来のドライバ差し替えに備えた防御。

## SQLite

- TEXT のバイト数は `OCTET_LENGTH(text)` で取る。`LENGTH(text)` は文字数を返す。`internal/core/db_query.go` の `sessionRowSelect` は `show` が出す本文量に合わせるため、sidechain を除外して `OCTET_LENGTH(m.content)` を集計する。
- SQLite には `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` がない。migration は `PRAGMA table_info(<table>)` で列の有無を先に確認し、失敗時も再度 state を見て成功扱いにできるか判断する。`internal/core/db_schema.go` の `ensureSessionsRepoPathColumn` / `ensureSessionsProjectDirColumnDropped` / `tableColumnPresent` がこの方針。
