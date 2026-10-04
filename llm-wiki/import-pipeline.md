---
regen: compiled
sources:
  - docs/rules/scope.md
  - docs/specs/jsonl-schema.md
  - cmd/somniloq/import.go
  - internal/core/import.go
  - internal/core/import_codex.go
  - internal/core/db.go
  - internal/core/db_write.go
  - internal/core/db_schema.go
  - internal/ingest/ingest.go
  - internal/ingest/process.go
  - internal/ingest/claudecode/adapter.go
  - internal/ingest/claudecode/jsonl.go
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

1. `cmd/somniloq/import.go` が `--source`, `--full`, `--yes` を処理し、確認後に `core.Import` を呼ぶ。
2. `internal/core/import.go` の `importSourceSpecs` が source と adapter と scan root を結びつける。source 追加時はここ、cmd の default directory wiring、仕様を同時に見る。CLI の source 表示文字列は `ImportSourceChoices()` から導出する。
3. `internal/core/import.go` が JSONL path の `ScanFiles` と source ごとの入口を担当する。Codex は `internal/ingest/codex/group.go` の `BuildGroups` が同本人の複数 rollout を相対 path・物理行順で集め、payload ID の重複判定と本人発言番号を決める。`internal/core/import_codex.go` の `importCodexGroups` が content hash と既存 cursor を照合し、変化した group の本文・関係・cursor を一 transaction で置換する。
4. Claude Code / Cursor Agent のファイル単位処理と、Codex の snapshot 読み取りには `internal/ingest/process.go` の `ProcessJSONL` を使う。line iteration、`Flush`、`import_state` の共通経路はここ。
5. source 固有の adapter が `FileHandler` として JSONL を解釈し、`ingest.NormalizedRecord` / `NormalizedMessage` / `SessionMeta` に落とす。
6. `ingest.PersistMessage` が source 共通の保存順序（session upsert、空白のみの本文は message skip、非空本文は blocks・出自・番号とともに保存）を実行する。
7. SQLite 書き込みは `internal/core/db_write.go` の `importTx` が実装する `ingest.ImportTransaction` 越し。adapter から SQL を直接触らない。

## source 別の注意

- Claude Code: `internal/ingest/claudecode/adapter.go` が `custom-title` / `agent-name` を buffer し、body record があるファイルだけ `Flush` で反映する。拡張 interface は `claudecode.SessionMetaWriter`。
- Codex: `internal/ingest/codex/jsonl.go` と `adapter.go` で、最初の有効 metadata の本人 identity・明示親参照・継承境界、block と元 timestamp、物理行を確認する。境界未満の context と ordinal 欠落の所属不明本文は本人原文と区別する。`group.go` と `internal/core/import_codex.go` で同本人 rollout の canonical 再構築、競合時の既存 group 保護、cursor の一括保存を確認する。canonical 順序と重複規則は `docs/specs/v0.14.0-contract.md` が正本。
- Cursor Agent: `internal/ingest/cursoragent/adapter.go` が transcript path から session を導出し、offset 前の改行数を `Begin` で復元する。未知 metadata、path と物理行に基づく identity、再処理時の重複・順序は `docs/specs/jsonl-schema.md` の Cursor Agent 節を先に確認し、`internal/ingest/cursoragent/jsonl.go` を読む。

## parse / 正規化診断の経路

計上対象、表示件数、exit code の契約は `docs/rules/scope.md` の「エラー処理と取り込みサマリ（source 共通）」を参照する。

source 固有の adapter が `LineUnparsed` と行番号付きの診断を返し、`internal/ingest/process.go` の `ProcessJSONL` がファイル単位の `ProcessResult` にまとめる。`internal/core/import.go` の `importWithAdapter` はこれを `ImportResult` に集約し、`Import` が source 間の結果を `ImportResult.add` で合流させる。診断の上限制御を変える際は、ファイル側の `MaxUnparsedDiagnostics` と、取り込み全体へ集約する `ImportResult.addUnparsedDiagnostics` を併せて確認する。最終的な stderr 出力は `cmd/somniloq/import.go` を読む。

## 変更時のテスト入口

- source 共通の import 制御: `internal/core/import_test.go`
- Claude Code JSONL 形式: `internal/ingest/claudecode/jsonl_test.go`
- Codex JSONL 形式・本人 group と canonical 順序・SQLite 再構築: `internal/ingest/codex/jsonl_test.go`, `internal/ingest/codex/group_test.go`, `internal/core/codex_import_test.go`, `internal/core/import_codex_group_test.go`
- Cursor Agent の path / parser: `internal/ingest/cursoragent/adapter_test.go`, `internal/ingest/cursoragent/jsonl_test.go`
- Cursor Agent の fixture、差分取り込み・再処理: `internal/ingest/testdata/cursor-agent/README.md`, `internal/ingest/testdata/cursor-agent/cursor-agent.jsonl`, `internal/core/cursor_agent_import_test.go`
- CLI の確認プロンプトや summary: `cmd/somniloq/import_test.go`, `cmd/somniloq/import_source_test.go`
