# Step 2: 実装

## worktree チェック（worktree セッション時のみ）

Environment セクションの Primary working directory が `.claude/worktrees/` 配下のパスを指していることを確認する。メインリポジトリのパスを指していたら、作業を中断してユーザーに「Primary working directory がメインリポジトリを指しています。`claude -w` で再起動してください」と報告する。

チェック結果をユーザーに出力すること: 「worktree: true/false, primary_working_directory: <パス>, check: ok/ng」

## 実装前の確認

プラン承認後、実装に入る前に以下を確認する:
- `rules/` と `references/knowledge.md` を Read で読み、関連する知見がないか確認する
- プランで言及されたファイルを改めて Read で読む（計画時から変わっている可能性がある）
- 型定義・インターフェース等を実物で確認する（記憶に頼らない）

## 実装の原則

- **TDD**: `/tdd` スキルに従う。テストを先に書き、失敗を確認し（RED）、最小の実装で通し（GREEN）、リファクタリングする。1テスト→1実装の垂直スライスで進める
- **Go**: `go vet ./...` と `go test ./...` が通ることを確認する
