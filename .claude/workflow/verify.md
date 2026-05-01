# Verify

## Intent

変更が要求を満たし、既存挙動を壊していないことを、適切な証拠で確認する。

## Inputs

- 変更差分（`git diff`, `git diff --cached`）
- plan または要求
- 関連テスト、ビルド設定

## Decision Criteria

- 自動検証を優先する。ビルド・テスト・静的チェックで確認できるものは先に通す
- CLI 出力・対話的挙動・実 DB ファイル・JSONL ログへの副作用は、`bin/somniloq <args>` を Bash で叩いて確認する
- 対話プロンプトを持つコマンド（`backfill` など）は `--yes` 経路で自動的に確認する。プロンプト UX 自体を確認したい場合は手動またはユーザー依頼
- 再現が難しい / 環境依存（実 DB ファイル、特定の JSONL データ等）はユーザーに依頼する
- 検証不能な High-risk 変更は完了扱いにしない

## somniloq Verification

- 自動検証: `go build ./...` / `go test ./...` / `go vet ./...`
- CLI 動作: `bin/somniloq <args>` を Bash で叩く（新規・変更したコマンドの stdout / stderr / 終了コード / help 文言を見る）
- 破壊的処理（DELETE を含む `backfill` など）は、テスト用 DB を作って `--yes` 経路で挙動を確認する

## Acceptance

- 実行した検証と結果を説明できる
- 検証しなかった項目がある場合、その理由が説明できる
- ユーザー確認が必要な場合は通過している

## Stop Conditions

- 必須の検証が環境要因で実行できない
- CLI 挙動の確認が必要だがユーザー確認が未完了
- 検証結果が要求または仕様と矛盾する
