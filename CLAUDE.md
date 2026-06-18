# CLAUDE.md

## プロジェクト概要

somniloq は Claude Code のセッションログ（JSONL）を読み取り、SQLite に保存・検索する CLI ツール。詳細: docs/rules/mission.md

## ワークフロー入口

入口は依頼の形で 2 通り。

- **Goal（`/goal` または `goal-workflow` を明示指定）**: `goal-workflow` skill を入口にする。Goal は作業全体を 1 commit 単位へ分割し、各 commit で `.claude/workflow/default.md` 以下の phase workflow を回す。Goal 手順の正本は `.claude/workflow/goal.md`。`goal-workflow` skill はそのファイルを読んで進める。Goal 前提では都度確認を避けて自動進行し、止まるのは各 workflow の Stop Conditions だけ。
- **単発依頼**: `.claude/workflow/default.md` を最初に Read し、Intake 分類（Small / Normal / High-risk / Exploratory）から必要な phase ファイルへ進む。

```text
goal-workflow skill（Goal の入口）
└── goal.md（正本: commit slicing / Goal Review / branch / ff-merge）
    └── default.md（各 commit / 単発依頼の Intake・Routing）
        ├── investigate.md — Exploratory 用の事実集め
        ├── plan.md — 計画作成（省略可条件含む。plan mode は使わない）
        ├── implement.md — 実装
        ├── verify.md — 動作確認
        ├── review.md — リスクベースの review depth 選択
        ├── finish.md — コミット
        └── maintenance.md — L3、節目で呼ぶ構造棚卸し
```

各 phase ファイルは入る前に Read で読む（CLAUDE.md の要約で済ませない）。
plan mode（`EnterPlanMode` / `ExitPlanMode`）は使わない。計画は内部で立ててそのまま実装する。
不明点があれば止まってユーザーに確認。なければ自動進行。
単発依頼はコミットまで終えたら止まる（次のタスクはユーザー指示待ち）。Goal は完了したら止まる。

## ビルド・テストコマンド

```bash
go test ./...                                # 全テスト実行
go build -o bin/somniloq ./cmd/somniloq      # バイナリビルド
```

## Constraints / サブエージェント活用

メインコンテキストを汚さないために、skill 以外の場面でもサブエージェントを積極的に使う。

正例（subagent に出す）:

- 結果が膨らむ・複数ファイル横断・複数キーワードでファンアウトする調査は Explore サブエージェントに委譲する
- 互いに独立した調査タスクが複数ある場合は、同一ターンで複数 subagent を並列起動する

負例（main で直接やる）:

- ファイル 1〜2 個の中身を見ればわかる調査は main で Read する
- grep 1 回で済む確認は main で Bash する
- 関連する複数 grep は 1 つの subagent でまとめる（複数 subagent に分けない）

判断軸:

- 回数ではなく「結果の量」「全体像把握が要るか」「main コンテキストを汚すか」で判断する
- 1 サブエージェント = 1 タスクに絞り、焦点を明確にする
- Agent ツールの prompt には、worktree で作業中の場合「git worktree で作業している。作業ディレクトリは <Primary working directory のフルパス> であり、このパス配下のファイルを参照すること」と明記する

## Constraints / ユーザーを調査リソースに使う

CLI 出力や期待挙動が絡む調査・バグ修正では、コードから推測を重ねる前にユーザーに聞く選択肢を持つ。往復 1 回で答えが出ることが多い。

- 期待する出力フォーマット・引数の解釈・エラー時の振る舞いなど、仕様が曖昧な点はユーザーに直接確認する
- 実機で再現する不具合は、ユーザーに再現コマンド・出力を貼ってもらう方が早い

## Constraints / ユーザーへの質問

ユーザーに質問することになった場合は `~/.claude/resources/rules/asking-user.md` を Read してから質問を組み立てる。

## Constraints / ドキュメント管理

- 各情報の置き場所は 1 箇所に限定する（同じ情報を複数のドキュメントに書くと SSoT が崩れる）
- 情報配置の正本は `docs/rules/information-management.md`。docs/ または llm-wiki/ を編集する前に読む
- `.claude/`・`CLAUDE.md`（Claude 側）と `.agents/`・`AGENTS.md`（Codex 側）は、目的・制約・判断基準の方向性を揃える。subagent、review delegation、tool 呼び出し、skill / workflow の実行手順は各エージェントの仕組みに合わせてよい。`skills/project-risk-check/SKILL.md` は同じリスク観点を保つ。片方で方針や制約を変更したら、同じコミットで他方にも必要な範囲を反映する。
- 新しいスキルやファイルを作成したら、同じステップで settings.json 等への登録も行う
- 特定ソースを編集するときだけ必要な罠は、そのソースのコメントに残す。横断的な挙動・設計理解は `llm-wiki/` の作業地図に残す。単一の集約知見ファイルは作らない

## Constraints / 言語ルール

- コード（変数名・コメント）は英語
- ドキュメント（CLAUDE.md, docs/, llm-wiki/, backlog/, README 等）は日本語
- コミットメッセージは英語（Conventional Commits）
