# somniloq

somniloq は Claude Code / Codex のセッションログ（JSONL）を読み取り、SQLite に保存・検索する CLI ツール。詳細は `docs/rules/mission.md` を正とする。

## Information Sources

変更時の検証方法と必須gateは `docs/rules/verification.md` を正本とする。

- `docs/rules/`: プロダクト目的、スコープ、アーキテクチャ、制約
- `docs/specs/`: 振る舞い仕様
- `backlog/backlog.md`: 未着手・進行中の作業項目。現状は単一ファイルを正とする
- `docs/decisions/`: 後から理由を問われる判断
- `llm-wiki/`: AI が編纂する作業地図。正本ではなく、docs/・ソース・テストに負ける
- `docs/specs/jsonl-schema.md`: Claude Code / Codex JSONL の構造の参照情報。JSONL ingest を変更するときは、仕様照合先として確認する

必要な情報だけ読む。全ファイルを毎回読む必要はない。ただし判断に影響する可能性がある情報源は、推測で済ませず実物を確認する。

## Core Policies

- skill は ICAR（Intent / Constraints / Acceptance / Relevant）を基本形にする。細かい手順や長い観点は、必要に応じて別 md や `llm-wiki/` へ逃がす。
- 小さい変更に重い手続きを載せない。作業の大きさとリスクで plan / verify / review の深さを選ぶ。
- 理想は全体が綺麗な状態だが、各 plan では今回の変更範囲と直接の依存先/依存元を中心に見る。広い構造改善は `backlog/backlog.md` へ切り出すか、節目でユーザー起点の `maintenance-audit` skill を使う。
- 不明点が仕様、CLI 挙動、データ保持、削除方針に影響するならユーザーに確認する。
- 自分で確認できることは自分で確認する。ユーザー確認は、実機依存・観察が必要な挙動・ユーザーの期待出力が早い場合に限る。
- 仕様変更は `docs/rules/`、`docs/specs/`、`backlog/backlog.md` の適切な場所に同期する。`docs/specs/` とテストが矛盾したら、現在の要求・`docs/rules/`・`docs/decisions/` と照合して古い方を直す。
- 特定ソースを編集するときだけ必要な罠は、そのソースのコメントに残す。横断的な挙動・設計理解は `llm-wiki/` の作業地図に残す。単一の集約知見ファイルは作らない。
- 後から制約になる判断は、制約を `docs/rules/` / `docs/specs/` に、理由を `docs/decisions/` に残す。

## Skills

Codex 用のプロジェクトスキルは `.agents/skills/` に置く。グローバルスキルは `~/.agents/skills/` に置く。

主に使うスキル:

- `investigate`: 計画前の不明点を調査する
- `design-decision`: 設計判断の価値基準を当てる
- `diff-review`: 変更差分をリスクに応じてレビューする
- `maintenance-audit`: 複数タスク後の構造・負債を棚卸しする（light / deep を scope で指定）
- `commit`（グローバル）: Conventional Commits 形式でコミットする

独立した調査・レビュー・実装は subagent で並列化してよい。subagent に依頼するときは、作業ディレクトリ `/Users/ryota/Sources/ryotapoi/somniloq` を明記する。

## somniloq Constraints

- `cmd/somniloq` は CLI 入出力・フラグ解析・表示整形を担当し、DB 操作や JSONL パースを持たない。
- `internal/core` は JSONL パース・DB スキーマ管理・インポート・クエリを担当し、`cmd/somniloq` に依存しない。
- SQLite スキーマ、migration、`backfill`、DELETE を伴う処理、SQL 集約、JSONL 取り込み境界は High-risk として扱う。
- JSONL 由来の値は SQL プレースホルダ経由で扱い、文字列連結で SQL を組み立てない。
- CLI の stdout/stderr、TSV/Markdown 出力、exit code、確認プロンプトの変更はユーザー影響として扱う。
- 後方互換性のためだけの shim / deprecated / fallback 分岐を追加しない。
- `--no-verify` でフックをスキップしない。
- 明示的な指示なしに force push しない。

## Language

- コード・コメント・コミットメッセージ: 英語
- ドキュメント（`AGENTS.md`, `.agents/`, `docs/`, `llm-wiki/`, `backlog/`, README 等）: 日本語
