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
- 開発ワークフロー: rules/workflow.md
- 情報管理の原則（フォルダ構成・情報分類・SSoT）: rules/information-management.md

## 開発ワークフロー

IMPORTANT: 各ステップの詳細は rules/workflow.md に定義。ステップに入る前に該当セクションを Read で読むこと。

1. **計画** — rules/workflow.md「Step 1: 計画」を読んでから着手
2. **プランレビュー** — rules/workflow.md「Step 2: プランレビュー」に従う
3. **実装** — rules/workflow.md「Step 3: 実装」を読んでから着手
4. **実装レビュー** — rules/workflow.md「Step 4: 実装レビュー」に従う
5. **コミット** — rules/workflow.md「Step 5: コミット」に従う

## 開発スタイル

### サブエージェント活用

メインコンテキストを汚さないために、skill 以外の場面でもサブエージェントを積極的に使う。

- 調査・比較・コード探索は Explore サブエージェントに委譲する
- 独立した作業は並列でサブエージェントを起動する
- 1 サブエージェント = 1 タスクに絞り、焦点を明確にする
- Agent ツールの prompt には、worktree で作業中の場合「git worktree で作業している。作業ディレクトリは <Primary working directory のフルパス> であり、このパス配下のファイルを参照すること」と明記する

## ドキュメント管理

- 同じ情報を複数のドキュメントに書かない。各情報の置き場所は1箇所に限定する
- 新しいスキルやファイルを作成したら、同じステップで settings.json 等への登録も行う
- 技術的な知見・ハマりどころは references/knowledge.md に集約する

## 言語

コミットメッセージは英語（Conventional Commits）。ドキュメントは日本語の場合がある。コード（変数名、コメント）は英語で書く。

## デバッグ

バグ修正・デバッグ時は `/debug` スキルを使う。
