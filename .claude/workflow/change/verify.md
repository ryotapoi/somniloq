# Verify

## Intent

変更が要求を満たし、既存挙動を壊していないことを、適切な証拠で確認する。

## Inputs

- 変更差分（`git diff`, `git diff --cached`）
- plan または要求
- 関連テスト、ビルド設定、Preview / GUI 確認手段

## Decision Criteria

- 自動検証を優先する。ビルド・テスト・静的チェックで確認できるものは先に通す
- テスト可能な振る舞い変更や bug fix に unit test / regression test がない場合、検証未完了として扱う。例外は理由を明記する
- GUI / Preview / 実機依存 / 外部連携 / 時間・非同期の挙動は、まず自力で取れる証拠（ビルド・テスト・スナップショット・ログ等）で確認する
- それでもユーザーの観察・操作なしに確定できない挙動が残るなら、Stop Condition または残存リスクとして報告する
- 検証不能な High-risk 変更は完了扱いにしない

## Verification

<!-- slot: ビルド・テスト・実行・実画面確認の具体手段を書く（例: build_macos / test_macos、go build ./... / go test ./...、./gradlew verifyAll / verifyAllConnected、CLI なら bin/<tool> <args>、Preview 確認手段）。ユーザー確認が必要な領域（実機・権限・外部連携）も書く。 -->
- 通常: `go test ./...`
- CLI ビルド確認: `go build -o bin/somniloq ./cmd/somniloq`
- 静的チェックが有効な変更: `go vet ./...`
- CLI 挙動変更: `bin/somniloq <command>` を一時 DB / testdata / 小さい JSONL で実行し、stdout / stderr / 終了コードを確認する
- SQLite schema / migration / `backfill`: 旧形式 DB、空 DB、再実行性、DELETE 対象あり/なしを確認する
- ユーザー確認が必要な領域: 外部連携・実機 UI はなし
<!-- /slot -->

## Acceptance

- 実行した検証と結果を説明できる
- 追加・更新した unit test / regression test、または追加しなかった理由を説明できる
- 検証しなかった項目がある場合、その理由が説明できる
- ユーザー確認が完了条件に必要な場合は通過している。必須でない場合は、未確認の理由を残存リスクまたは Goal 完了報告のユーザー判断候補として説明できる。

## Stop Conditions

- 必須の検証が環境要因で実行できず、代替手段でも裏付けられない
- 完了判断に必須の UI / 挙動確認を、自力検証・代替証拠・ユーザー確認のいずれでも裏付けられない
- 検証結果が要求または仕様と矛盾する
