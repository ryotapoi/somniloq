---
name: somniloq-risk-check
description: Use for somniloq-specific plan or implementation checks when changes touch SQLite schema or migrations, backfill, JSONL import, CLI output semantics, search filters, or cmd/internal boundaries.
---

# somniloq Risk Check

## ICAR

- **Intent**: somniloq 固有の mission・アーキテクチャ制約・既知の落とし穴に照らして、計画または実装のリスクを確認する。
- **Constraints**:
  - 汎用レビューではなく、somniloq 固有の実害に絞る。
  - 仕様・CLI 挙動・データ保持・削除方針の判断が必要なら、実装判断として決めずユーザー確認に回す。
  - 具体的な過去知見は `references/knowledge.md` を参照し、skill 本体には増やしすぎない。
- **Acceptance**:
  - `LGTM` またはリスク一覧がある。
  - リスクには影響、根拠、推奨対応がある。
  - 必要な場合、更新すべき `rules/`, `specs/`, `backlog/backlog.md`, `decisions/`, `references/knowledge.md`, `references/jsonl-schema.md` が明確。
- **Relevant**:
  - ユーザー依頼、plan、または未コミット差分
  - `rules/mission.md`
  - `rules/scope.md`
  - `rules/architecture.md`
  - `rules/constraints.md`
  - `rules/information-management.md`
  - `references/knowledge.md`
  - `references/jsonl-schema.md`

## Checkpoints

- Mission の「Claude Code と Codex のセッションログを SQLite に保存・検索する CLI」から外れていないか。
- `cmd/somniloq → internal/core` の依存方向と責務境界を守っているか。
- SQLite schema / migration / `backfill` / DELETE の変更で、再実行性・既存 DB・確認プロンプト・非対話環境の扱いが検証されているか。
- SQL はプレースホルダを使い、JSONL 由来データを文字列連結で埋め込んでいないか。
- `modernc.org/sqlite` の既知の罠（`LastInsertId`, `:memory:` 接続、migration 判定など）を踏んでいないか。
- Claude Code / Codex JSONL の形式差、未知フィールド、メタのみセッション、空 text、差分取り込みキーを壊していないか。
- CLI の stdout/stderr、TSV/Markdown、exit code、usage/help、確認プロンプトが既存仕様と同期しているか。
- `--project`, `--since`, `--until`, `--summary`, `--short` など検索・表示オプションの意味を意図せず変えていないか。
- README、`rules/scope.md`、`references/jsonl-schema.md`、テストが実装差分と矛盾していないか。
