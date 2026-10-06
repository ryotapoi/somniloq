---
regen: compiled
sources:
  - docs/rules/scope.md
  - docs/specs/jsonl-schema.md
  - cmd/somniloq/import.go
  - cmd/somniloq/config.go
  - internal/core/import.go
  - internal/core/import_codex.go
  - internal/core/import_claude.go
  - internal/core/db.go
  - internal/core/db_write.go
  - internal/core/db_schema.go
  - internal/ingest/ingest.go
  - internal/ingest/process.go
  - internal/ingest/claudecode/adapter.go
  - internal/ingest/claudecode/jsonl.go
  - internal/ingest/claudecode/snapshot.go
  - internal/ingest/codex/adapter.go
  - internal/ingest/codex/group.go
  - internal/ingest/codex/jsonl.go
  - internal/ingest/cursoragent/adapter.go
  - internal/ingest/cursoragent/jsonl.go
  - internal/ingest/cursoragent/adapter_test.go
  - internal/ingest/cursoragent/jsonl_test.go
  - internal/core/cursor_agent_import_test.go
  - internal/ingest/codex/group_test.go
  - internal/core/import_codex_group_test.go
  - internal/ingest/testdata/cursor-agent/README.md
  - internal/ingest/testdata/cursor-agent/cursor-agent.jsonl
---

# Import pipeline

JSONL 取り込みを変えるときの読む順序。仕様そのものは `docs/rules/scope.md` と `docs/specs/jsonl-schema.md` が正本で、このページは実装の経路だけを示す。

## 経路

1. `cmd/somniloq/import.go` が入力の選択と確認を行い、`core.Import` を呼ぶ。設定から入力を渡す経路は `cmd/somniloq/config.go` も確認する。
2. `internal/core/import.go` の `importSourceSpecs` と `Import` が source の選択・専用入口への振り分けを担当する。CLI の source 表示文字列は `ImportSourceChoices()` から導出する。
3. Codex は `internal/ingest/codex/group.go` の `BuildGroups` → `internal/core/import_codex.go` の `importCodexGroups`、Claude Code は `internal/ingest/claudecode/snapshot.go` の `BuildSnapshots` → `internal/core/import_claude.go` の `importClaudeSnapshots` を追う。解析した本人全体と既存 cursor の照合、保存 transaction の境界はそれぞれの core 入口を見る。
4. Cursor Agent は `importWithAdapter` → `internal/ingest/process.go` の `ProcessJSONL`。ファイル単位の差分再開、`FileHandler` / `Flush` と `import_state` の更新はこの経路を確認する。
5. source 固有の parser と正規化型は `internal/ingest/`、SQLite 保存は `internal/core/db_write.go` の `importTx`。共通の本文保存順序は `ingest.PersistMessage`、Codex / Claude の本人置換は各 core 入口から辿る。

## source 別の注意

- Claude Code: `adapter.go` の走査から `snapshot.go` の `BuildSnapshots` へ進む。本人・直接親・root 所属、title / agent-name、cwd の解決再利用は snapshot 側、本文の不変判定・置換と cursor の競合確認は `internal/core/import_claude.go` を見る。通常取り込みを adapter の `Flush` 経路と混同しない。
- Codex: `internal/ingest/codex/jsonl.go` と `adapter.go` で、最初の有効 metadata の本人 identity・明示親参照・継承境界、block と元 timestamp、物理行を確認する。境界未満の context と ordinal 欠落の所属不明本文は本人原文と区別する。`group.go` と `internal/core/import_codex.go` で同本人 rollout の canonical 再構築、競合時の既存 group 保護、cursor の一括保存を確認する。canonical 順序と重複規則は `docs/specs/v0.14.0-contract.md` が正本。
- Cursor Agent: `internal/ingest/cursoragent/adapter.go` が transcript path から session を導出し、offset 前の改行数を `Begin` で復元する。未知 metadata、path と物理行に基づく identity、再処理時の重複・順序は `docs/specs/jsonl-schema.md` の Cursor Agent 節を先に確認し、`internal/ingest/cursoragent/jsonl.go` を読む。

## parse / 正規化診断の経路

計上対象、表示件数、exit code の契約は `docs/rules/scope.md` の「エラー処理と取り込みサマリ（source 共通）」を参照する。

Cursor Agent は adapter の `LineUnparsed` → `ProcessJSONL` の `ProcessResult` → `importWithAdapter` の `ImportResult` を辿る。Codex は `BuildGroups` と `importCodexGroups`、Claude Code は `BuildSnapshots` と `importClaudeSnapshots` で解析・読み取り診断を集約する。取り込み全体の合流と上限制御は `ImportResult.add` / `addUnparsedDiagnostics`、最終的な stderr 出力は `cmd/somniloq/import.go` を確認する。

## 変更時のテスト入口

- source 共通の import 制御: `internal/core/import_test.go`
- Claude Code JSONL 形式・snapshot・本人置換: `internal/ingest/claudecode/jsonl_test.go`, `internal/ingest/claudecode/snapshot_test.go`, `internal/core/import_claude_test.go`
- Codex JSONL 形式・本人 group と canonical 順序・SQLite 再構築: `internal/ingest/codex/jsonl_test.go`, `internal/ingest/codex/group_test.go`, `internal/core/codex_import_test.go`, `internal/core/import_codex_group_test.go`
- Cursor Agent の path / parser: `internal/ingest/cursoragent/adapter_test.go`, `internal/ingest/cursoragent/jsonl_test.go`
- Cursor Agent の fixture、差分取り込み・再処理: `internal/ingest/testdata/cursor-agent/README.md`, `internal/ingest/testdata/cursor-agent/cursor-agent.jsonl`, `internal/core/cursor_agent_import_test.go`
- CLI の確認プロンプトや summary: `cmd/somniloq/import_test.go`, `cmd/somniloq/import_source_test.go`
