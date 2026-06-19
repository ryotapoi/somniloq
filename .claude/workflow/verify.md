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
- 自動検証: `go build ./...` / `go test ./...` / `go vet ./...`
- CLI 動作: `bin/somniloq <args>` を Bash で叩く（新規・変更したコマンドの stdout / stderr / 終了コード / help 文言を見る）
- 破壊的処理（DELETE を含む `backfill` など）は、テスト用 DB を作って `--yes` 経路で挙動を確認する
- ユーザー確認が必要な領域: 実 DB ファイル・特定の JSONL データなど環境依存で再現が難しいもの、対話プロンプト UX 自体の確認
<!-- /slot -->

## Acceptance

- 実行した検証と結果を説明できる
- 追加・更新した unit test / regression test、または追加しなかった理由を説明できる
- 検証しなかった項目がある場合、その理由が説明できる
- ユーザー確認が必要な場合は通過している

## Stop Conditions

- 必須の検証が環境要因で実行できない
- UI / 挙動確認が必要だがユーザー確認が未完了
- 検証結果が要求または仕様と矛盾する
