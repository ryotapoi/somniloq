# Verify Workflow

## ICAR

- **Intent**: 変更が要求を満たし、既存挙動を壊していないことを、適切な証拠で確認する。
- **Constraints**:
  - 自動検証を優先する。ビルド・テスト・静的チェックで確認できるものは先に通す。
  - CLI 出力、exit code、DB 変化を自分で確認できる場合は実際に確認する。
  - ユーザー確認は、ユーザー環境の実データや期待出力の判断が必要な場合に限る。
  - 検証不能な High-risk 変更は完了扱いにしない。
- **Acceptance**:
  - 実行した検証と結果を説明できる。
  - 検証しなかった項目がある場合、その理由が説明できる。
  - ユーザー確認が必要なものだけ依頼し、不要なものは理由を説明できる。
- **Relevant**:
  - 変更差分（`git diff`, `git diff --cached`）
  - plan または要求
  - 関連テスト、ビルド設定、CLI コマンド、SQLite DB 状態

## somniloq Verification

- 通常: `go test ./...`
- CLI ビルド確認: `go build -o bin/somniloq ./cmd/somniloq`
- 静的チェックが有効な変更: `go vet ./...`
- CLI 挙動変更: `bin/somniloq <command>` を一時 DB / testdata / 小さい JSONL で実行し、stdout/stderr/exit code を確認する。
- SQLite schema / migration / `backfill`: 旧形式 DB、空 DB、再実行性、DELETE 対象あり/なしを確認する。
- JSONL import: Claude Code と Codex の形式差、差分取り込み、メタのみセッション、空 text、未知フィールドを確認する。

## User Check

- docs / テストのみ / 内部ロジックのみの変更では不要。
- 実ユーザーのローカルログ、巨大 DB、期待出力の好み、実機依存の観察が必要な場合だけ依頼する。

## Stop Conditions

- 必須の検証が環境要因で実行できない。
- ユーザー確認が必要だが未完了。
- 検証結果が要求または仕様と矛盾する。
