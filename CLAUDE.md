# CLAUDE.md

## プロジェクト概要

somniloq は Claude Code のセッションログ（JSONL）を読み取り、SQLite に保存・検索する CLI ツール。詳細: rules/mission.md

## ICAR

- **Intent**: プラン → 実装 → 動作確認 → レビュー → コミット を、各ステップで ICAR を満たした状態で進める
- **Constraints**:
  - 各ステップに入る前に該当する `rules/workflow/*.md` を Read で読む（CLAUDE.md の要約で済ませない）
  - 不明点があれば止まってユーザーに確認する。それ以外は自動進行
  - コミットまで終えたら止まる。次のタスクはユーザー指示を待つ
  - コミットは workflow 内外を問わず必ず `/commit` スキル経由
  - コミットメッセージは英語（Conventional Commits）。コード（変数名・コメント）は英語。ドキュメントは日本語可
- **Acceptance**: コミット完了 + 動作確認通過 + `/review-code-all` 通過
- **Relevant**: rules/mission.md, rules/scope.md, rules/architecture.md, rules/constraints.md, rules/workflow/, rules/information-management.md

## ビルド・テストコマンド

```bash
go test ./...                                # 全テスト実行
go build -o bin/somniloq ./cmd/somniloq      # バイナリビルド
```

## 開発ワークフロー

各ステップの詳細ファイルを、**ステップに入る前**に Read で読むこと。

1. **計画（プランモード）**
   1. **UX シナリオ** — rules/workflow/1a-ux-scenario.md
   2. **調査・設計判断** — rules/workflow/1b-design.md
   3. **プラン作成・レビュー** — rules/workflow/1c-plan.md
2. **実装**
   1. **実装** — rules/workflow/2a-implement.md
   2. **動作確認** — rules/workflow/2b-verify.md
   3. **レビュー** — rules/workflow/2c-review.md
3. **コミット** — rules/workflow/3-finish.md

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
