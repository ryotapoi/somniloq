---
regen: compiled
sources:
  - docs/rules/scope.md
  - docs/specs/jsonl-schema.md
  - cmd/somniloq/import.go
  - internal/core/import.go
  - internal/core/db.go
  - internal/core/db_write.go
  - internal/ingest/ingest.go
  - internal/ingest/process.go
  - internal/ingest/claudecode/adapter.go
  - internal/ingest/claudecode/jsonl.go
  - internal/ingest/codex/adapter.go
  - internal/ingest/codex/jsonl.go
---

# Import pipeline

JSONL 取り込みを変えるときの読む順序。仕様そのものは `docs/rules/scope.md` と `docs/specs/jsonl-schema.md` が正本で、このページは実装の経路だけを示す。

## 経路

1. `cmd/somniloq/import.go` が `--source`, `--full`, `--yes` を処理し、確認後に `core.Import` を呼ぶ。
2. `internal/core/import.go` の `importSourceSpecs` が source と adapter と scan root を結びつける。source 追加時はここ、cmd の default directory wiring、仕様を同時に見る。CLI の source 表示文字列は `ImportSourceChoices()` から導出する。
3. `importWithAdapter` が `ScanFiles`、`import_state`、ファイルサイズ比較、offset 決定、`adapter.ProcessFile` を担当する。
4. `internal/ingest/process.go` の `ProcessJSONL` が共通 skeleton。transaction、line iteration、`Flush`、`import_state` 更新はここ。
5. source 固有の adapter が `FileHandler` として JSONL を解釈し、`ingest.NormalizedRecord` / `NormalizedMessage` / `SessionMeta` に落とす。
6. `ingest.PersistMessage` が source 共通の保存順序（session upsert、空 content は message skip、非空 content は message insert）を実行する。
7. SQLite 書き込みは `internal/core/db_write.go` の `importTx` が実装する `ingest.ImportTransaction` 越し。adapter から SQL を直接触らない。

## source 別の注意

- Claude Code: `internal/ingest/claudecode/adapter.go` が `custom-title` / `agent-name` を buffer し、body record があるファイルだけ `Flush` で反映する。拡張 interface は `claudecode.SessionMetaWriter`。
- Codex: `internal/ingest/codex/adapter.go` の `Begin` が offset 前の prefix から `session_meta` を復元する。差分取り込みで追記分だけ読むと meta を失うため。
- Codex の message UUID は `internal/ingest/codex/jsonl.go` の path + line number。line number は blank line も数える。
- `LineUnparsed` は壊れた JSON / malformed payload の計上用。未知 type や意図的に無視する record は `LineIgnored`。

## 変更時のテスト入口

- source 共通の import 制御: `internal/core/import_test.go`
- Claude Code JSONL 形式: `internal/ingest/claudecode/jsonl_test.go`
- Codex JSONL 形式・差分 meta 復元: `internal/ingest/codex/jsonl_test.go`, `internal/core/codex_import_test.go`
- CLI の確認プロンプトや summary: `cmd/somniloq/import_test.go`, `cmd/somniloq/import_source_test.go`
