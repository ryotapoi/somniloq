# CLAUDE.md

## プロジェクト概要

somniloq は Claude Code のセッションログ（JSONL）を読み取り、SQLite に保存・検索する CLI ツール。詳細: rules/mission.md

## ワークフロー入口

タスクを始める時は `.claude/workflow/default.md` を最初に Read する。
そこから Intake 分類（Small / Normal / High-risk / Exploratory）を判定し、必要な phase ファイルへ進む。

phase ファイル一覧（`.claude/workflow/`）:

- `default.md` — 入口、Intake、Routing
- `investigate.md` — Exploratory 用の事実集め
- `plan.md` — 計画作成（省略可条件含む）
- `implement.md` — 実装
- `verify.md` — 動作確認
- `review.md` — リスクベースの review depth 選択
- `finish.md` — コミット
- `maintenance.md` — L3、節目で呼ぶ構造棚卸し

各 phase ファイルは入る前に Read で読む（CLAUDE.md の要約で済ませない）。
不明点があれば止まってユーザーに確認。なければ自動進行。
コミットまで終えたら止まる。次のタスクはユーザー指示待ち。

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

## Constraints / ドキュメント管理

- 各情報の置き場所は 1 箇所に限定する（同じ情報を複数のドキュメントに書くと SSoT が崩れる）
- 新しいスキルやファイルを作成したら、同じステップで settings.json 等への登録も行う
- 技術的な知見・ハマりどころは references/knowledge.md に集約する

## Constraints / 言語ルール

- コード（変数名・コメント）は英語
- ドキュメント（CLAUDE.md, rules/, references/, decisions/, backlog/, README 等）は日本語
- コミットメッセージは英語（Conventional Commits）
