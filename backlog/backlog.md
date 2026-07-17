# Backlog

## v0.8.1（2026-07-17 maintenance-audit より・内部のみの変更）

- [ ] `--help` 事前判定の flag メタデータ二重管理を解消する
  - `cmd/somniloq/main.go` の `isHelpRequest` / `configCommandFlagConsumesValue` が各 command の `FlagSet` 宣言を手動複製している。flag 追加時に片側を忘れると、壊れた config がある環境で help だけ表示できなくなる
  - 単一宣言源化か、完全な同期契約テストかは design-decision 対象
- [ ] 未使用の `Adapter.Source()` を削除する
  - `internal/ingest/ingest.go` の `Adapter` interface が `Source()` を要求しているが、呼び出しはテスト含め 0 件。実際の source は `ProcessJSONL` / `importSourceSpec` 側で決まっており、source identity が二重化している

## v0.9.0（2026-07-17 maintenance-audit より・エラーメッセージが変わる）

- [ ] DB query error に操作文脈を付ける
  - `internal/core/db_query.go` の public query 境界（`ListSessions` 等）が driver error をそのまま返している。DB 破損・lock・schema 変更時にどの query が失敗したか切り分けにくい
  - 操作名を付けつつ `%w` と既存の `sql.ErrNoRows` 契約を維持する
