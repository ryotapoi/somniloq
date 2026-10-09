# somniloq

somniloq は複数の coding agent のセッションログ（JSONL）を SQLite に保存・検索する CLI。目的と非目標は [要件](docs/requirements.md) に置く。

## タスク別の入口

作業開始時に [開発時の取り決め](docs/development.md) を読む。その他はタスクに必要な文書だけを読み、判断に影響する実体を推測で済ませない。

- 目的、非目標、保持制約: `docs/requirements.md`
- 横断する設計判断とその理由: `decisions/` の `status: current` の ADR
- CLI の参照・出力形式: `docs/cli-contract.md`。利用方法は README と help
- JSONL 入力形状: `docs/jsonl-format.md`。版別の観測は `cache/` として時点を確認する
- 旧 SQLite 履歴の移行: `docs/migration.md` と ADR 0026
- 開発時の言語・操作: `docs/development.md`
- 検証方法と必須 gate: `docs/verification.md`
- 版を切る作業: `docs/release.md`
- `docs/`、`backlog/`、`decisions/`、`cache/` の変更: `docs/information-management.md`
- 過去の判断の理由: `decisions/` の `status: superseded` の ADR
