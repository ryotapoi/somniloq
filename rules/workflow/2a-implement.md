# Step 2a: 実装

## worktree チェック

Primary working directory（Environment セクション）が `.claude/worktrees/` 配下を指している場合のみ、このセクションを実行する。メインリポジトリ配下を指している場合はスキップする。

Primary working directory と CWD（`pwd` などで得られる現在のカレントディレクトリ）が一致していることを確認する。Primary working directory は `.claude/worktrees/` 配下なのに CWD がそこと一致しない場合、作業を中断してユーザーに「Primary working directory と CWD が乖離しています（primary: <パス>, cwd: <パス>）。`claude -w` で再起動してください」と報告する。

チェック結果をユーザーに出力すること: 「primary_working_directory: <パス>, cwd: <パス>, check: ok/ng/skip」

## 実装前の確認

プラン承認後、実装に入る前に以下を確認する:
- `rules/` と `references/knowledge.md` を Read で読み、関連する知見がないか確認する
- プランで言及されたファイルを改めて Read で読む（計画時から変わっている可能性がある）
- 型定義・インターフェース等を実物で確認する（記憶に頼らない）

## 実装の原則

- **TDD**: `/tdd` スキルに従う。テストを先に書き、失敗を確認し（RED）、最小の実装で通し（GREEN）、リファクタリングする。1テスト→1実装の垂直スライスで進める
- **Go**: `go vet ./...` と `go test ./...` が通ることを確認する

## 実装が完了したら

次のサブステップに進む: rules/workflow/2b-verify.md を読む。
