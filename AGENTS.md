# somniloq

somniloq は複数の coding agent のセッションログ（JSONL）を SQLite に保存・検索する CLI。目的と非目標は [docs/rules/mission.md](docs/rules/mission.md) を正本とする。

## タスク別の正本

タスクに必要な文書だけを読む。推測で済ませず、判断に影響する正本を確認する。

- 目的、非目標、対象範囲、CLI の表面を変更する: `docs/rules/mission.md` と `docs/rules/scope.md`
- 責務配置や依存方向を変更する: `docs/rules/architecture.md`
- SQLite schema、migration、`backfill`、DELETE、SQL 集約、JSONL ingest を変更する: `docs/rules/constraints.md` と `docs/rules/verification.md`。JSONL ingest 時は `docs/specs/jsonl-schema.md` も確認する
- 検証方法と必須 gate: `docs/rules/verification.md`
- 振る舞い仕様: `docs/specs/`
- `docs/`、`backlog/`、`llm-wiki/` を変更する: `docs/rules/information-management.md`
- 過去の判断の理由が必要なとき: `docs/decisions/`

## Language

コード・コメント・コミットメッセージは英語、`AGENTS.md`・`.agents/`・`docs/`・`llm-wiki/`・`backlog/`・README 等の文書は日本語で書く。
