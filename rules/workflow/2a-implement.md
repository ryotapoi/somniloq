# Step 2a: 実装

## ICAR

- **Intent**: プラン承認後、最新の前提を確認した上で TDD でコードを書き、Go ツールチェーンを通す
- **Constraints**:
  - **TDD**: `/tdd` スキルに従う
  - **Go**: `go vet ./...` と `go test ./...` が通ることを確認する
  - 実装前に `rules/` と `references/knowledge.md` を読む（関連知見の確認）
  - プランで言及されたファイルを改めて Read で読む（計画時から変わっている可能性がある）
  - 型定義・インターフェース等は実物で確認する（記憶に頼らない）
- **Acceptance**:
  - worktree チェック: Primary working directory が `.claude/worktrees/` 配下を指す場合、Primary working directory == CWD
  - 実装が完了し、`go vet` と `go test` が通っている状態
- **Relevant**: rules/, references/knowledge.md, `/tdd` スキル

## worktree チェック

Primary working directory（Environment セクション）が `.claude/worktrees/` 配下を指している場合のみ実施する。メインリポジトリ配下なら skip。

不一致時の対処: 作業を中断してユーザーに「Primary working directory と CWD が乖離しています（primary: <パス>, cwd: <パス>）。`claude -w` で再起動してください」と報告する。

チェック結果をユーザーに出力する: 「primary_working_directory: <パス>, cwd: <パス>, check: ok/ng/skip」
