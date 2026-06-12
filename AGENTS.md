# somniloq

somniloq は Claude Code / Codex のセッションログ（JSONL）を読み取り、SQLite に保存・検索する CLI ツール。詳細は `rules/mission.md` を正とする。

## Entry Point

入口は依頼の形で 2 通り。

- **Goal（`/goal` または `goal-workflow` を明示指定）**: グローバルの `goal-workflow` skill（`~/.agents/skills/goal-workflow/`）を入口にする。Goal は作業全体を 1 commit 単位へ分割し、各 commit で `.agents/workflow/default.md` 以下の phase workflow を回す。Goal 手順の正本は `.agents/workflow/goal.md`。
- **単発依頼**: 最初に `.agents/workflow/default.md` を読み、Intake から必要な phase ファイルへ進む。

各 phase に入るときだけ、対応する workflow ファイルを読む。`AGENTS.md` の要約だけで進めない。

```text
goal-workflow skill（グローバル / Goal の入口）
└── .agents/workflow/goal.md（正本: commit slicing / Claude review / 完了条件）
    └── default.md（各 commit / 単発依頼の Intake・Routing）
        ├── investigate.md
        ├── plan.md
        ├── implement.md
        ├── verify.md
        ├── review.md
        ├── finish.md
        └── maintenance.md
```

Claude Code 由来の `.claude/` は参考資料として扱ってよいが、Codex の入口は `AGENTS.md` と `.agents/` に統一する。

## Information Sources

- `rules/`: プロダクト目的、スコープ、アーキテクチャ、制約
- `specs/`: 振る舞い仕様。現状は未配置だが、テストだけでは意図が残らない仕様が増えたら追加する
- `backlog/backlog.md`: 未着手・進行中の作業項目。現状は単一ファイルを正とする
- `decisions/`: 後から理由を問われる判断
- `references/knowledge.md`: 技術的な知見・ハマりどころ
- `references/jsonl-schema.md`: Claude Code / Codex JSONL の参照情報

必要な情報だけ読む。全ファイルを毎回読む必要はない。ただし判断に影響する可能性がある情報源は、推測で済ませず実物を確認する。

## Core Policies

- workflow / skill は ICAR（Intent / Constraints / Acceptance / Relevant）を基本形にする。細かい手順や長い観点は、必要に応じて workflow 内の phase ICAR、別 md、`references/knowledge.md` へ逃がす。
- 小さい変更に重い手続きを載せない。作業の大きさとリスクで plan / verify / review の深さを選ぶ。
- 原則 1 plan = 1 commit。独立した成果が混ざるなら plan を分ける。
- 理想は全体が綺麗な状態だが、各 plan では今回の変更範囲と直接の依存先/依存元を中心に見る。広い構造改善は必要に応じて `backlog/backlog.md` または `maintenance.md` へ切り出す。
- 不明点が仕様、CLI 挙動、データ保持、削除方針に影響するならユーザーに確認する。
- 自分で確認できることは自分で確認する。ユーザー確認は、実機依存・観察が必要な挙動・ユーザーの期待出力が早い場合に限る。
- 仕様変更は `rules/`、`specs/`、`backlog/backlog.md` の適切な場所に同期する。`specs/` とテストが矛盾したら、現在の要求・`rules/`・`decisions/` と照合して古い方を直す。
- 技術的知見は `references/knowledge.md` に集約する。
- 後から制約になる判断は `decisions/` に残す。
- workflow は 1 つの commit 単位で回す。Goal が複数 commit に分かれる場合は `goal-workflow` skill に従って commit 単位へ分けて繰り返す。
- 単発依頼はコミットまで終えたら止まる（次のタスクはユーザー指示待ち）。Goal は完了したら止まる。
- `.claude/`・`CLAUDE.md`（Claude 側）と `.agents/`・`AGENTS.md`（Codex 側）は、目的・制約・判断基準の方向性を揃える。subagent、review delegation、tool 呼び出し、skill / workflow の実行手順は各エージェントの仕組みに合わせてよい。`skills/somniloq-risk-check/SKILL.md` は同じリスク観点を保つ。片方で方針や制約を変更したら、同じコミットで他方にも必要な範囲を反映する。

## Skills

Codex 用のプロジェクトスキルは `.agents/skills/` に置く。グローバルスキルは `~/.agents/skills/` に置く。`goal-workflow` はグローバルスキルを使う。

主に使うスキル:

- `goal-workflow`（グローバル）: `/goal` または明示指定時だけ使う。Goal を 1 commit 単位へ分割して完了まで進める
- `investigate`: 計画前の不明点を調査する
- `design-decision`: 設計判断の価値基準を当てる
- `change-review`: 変更差分をリスクに応じてレビューする
- `maintenance-audit`: 複数タスク後の構造・負債を棚卸しする（light / deep を scope で指定）
- `somniloq-risk-check`: somniloq 固有の制約に照らして確認する
- `commit`: Conventional Commits 形式でコミットする

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

## Tooling

```bash
go test ./...                                # 全テスト実行
go build -o bin/somniloq ./cmd/somniloq      # CLI バイナリビルド
go vet ./...                                 # 静的チェック
```

フォーマットは `gofmt` / `goimports` を使う。Codex hook は Go ファイル編集後に `.codex/hooks/go-format.sh` を実行する。

## Language

- コード・コメント・コミットメッセージ: 英語
- ドキュメント（`AGENTS.md`, `.agents/`, `rules/`, `backlog/`, `decisions/`, `references/`, README 等）: 日本語
