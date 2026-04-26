# CLAUDE.md

## プロジェクト概要

somniloq は Claude Code のセッションログ（JSONL）を読み取り、SQLite に保存・検索する CLI ツール。詳細: rules/mission.md

## ビルド・テストコマンド

```bash
go test ./...                          # 全テスト実行
go test ./internal/...                 # 特定パッケージのテスト実行
go test ./internal/... -run TestImport # 特定テストのみ実行
go build -o bin/somniloq ./cmd/somniloq      # バイナリビルド
```

## rules/

計画・実装時に必ず Read で参照すること。CLAUDE.md の要約で済ませず、実ファイルを読んで判断する。

- プロダクト目的・非目標: rules/mission.md
- 主要機能・CLI・テーブル設計: rules/scope.md
- モジュール構成と依存方向: rules/architecture.md
- 技術的制約・禁止事項: rules/constraints.md
- 開発ワークフロー: rules/workflow/
- 情報管理の原則（フォルダ構成・情報分類・SSoT）: rules/information-management.md

## 開発ワークフロー

IMPORTANT: 各フェーズに入る時点で、そのフェーズに含まれる詳細ファイルをまとめて Read で読むこと。CLAUDE.md の要約で済ませず、毎回実ファイルを読む。

- **計画フェーズ着手時**: rules/workflow/1a-ux-scenario.md / 1b-design.md / 1c-plan.md をまとめて Read してから 1a に入る
- **実装フェーズ着手時**: rules/workflow/2a-implement.md / 2b-verify.md / 2c-review.md / 3-finish.md をまとめて Read してから 2a に入る（実装〜コミットはセッションを分断しないため、3 もここで一緒に読む）
- 計画と実装はセッションを分断するため、計画フェーズで 2a 以降を先読みしない

1. **計画（プランモード）**
   1. **UX シナリオ** — 1a-ux-scenario.md
   2. **調査・設計判断** — 1b-design.md
   3. **プラン作成・レビュー** — 1c-plan.md
2. **実装**
   1. **実装** — 2a-implement.md
   2. **動作確認** — 2b-verify.md
   3. **レビュー** — 2c-review.md
3. **コミット** — 3-finish.md

### 進行の原則

- **不明点があれば止まって確認する。** 仮定で進めない
- **不明点がなければ workflow を続ける。** 自動進行がデフォルト。ワークフロー詳細に明示的な停止指示がある場合のみ止まる
- **コミットまで終えたら止まる。** 次のタスクは勝手に始めない

## 開発スタイル

### サブエージェント活用

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
- 複数 subagent の並列起動は、論理的に独立したタスクが同時にあるときだけ行う
- 1 サブエージェント = 1 タスクに絞り、焦点を明確にする
- Agent ツールの prompt には、worktree で作業中の場合「git worktree で作業している。作業ディレクトリは <Primary working directory のフルパス> であり、このパス配下のファイルを参照すること」と明記する

### ユーザーを調査リソースに使う

CLI 出力や期待挙動が絡む調査・バグ修正では、コードから推測を重ねる前にユーザーに聞く選択肢を持つ。往復 1 回で答えが出ることが多い。

- 期待する出力フォーマット・引数の解釈・エラー時の振る舞いなど、仕様が曖昧な点は仮定せずユーザーに直接確認する
- 実機で再現する不具合は、ユーザーに再現コマンド・出力を貼ってもらう方が早い

## ドキュメント管理

- 同じ情報を複数のドキュメントに書かない。各情報の置き場所は1箇所に限定する
- 新しいスキルやファイルを作成したら、同じステップで settings.json 等への登録も行う
- 技術的な知見・ハマりどころは references/knowledge.md に集約する

## コミット

コミットは workflow 内外を問わず、必ず `/commit` スキルを使う。直接 `git commit` を打たない。

## 言語

コミットメッセージは英語（Conventional Commits）。ドキュメントは日本語の場合がある。コード（変数名、コメント）は英語で書く。
