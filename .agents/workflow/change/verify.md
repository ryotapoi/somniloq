# Verify Workflow

正本は `~/.config/agents/workflow/change/verify.md`。これを Read し、以下のプロジェクト固有の検証手段を使って確認する。

## Verification

<!-- slot: ビルド・テスト・実行・実画面確認の具体手段を書く（例: build_macos / test_macos、go build ./... / go test ./...、./gradlew verifyAll / verifyAllConnected、CLI なら bin/<tool> <args>、Preview 確認手段）。テスト構成上の注意（SPM dependent package など）や API 仕様の一次情報確認手段も書く（外部 API が無ければ「N/A — 外部 API 参照なし」と明記する）。 -->
- 通常: `go test ./...`
- CLI ビルド確認: `go build -o bin/somniloq ./cmd/somniloq`
- 静的チェックが有効な変更: `go vet ./...`
- CLI 挙動変更: `bin/somniloq <command>` を一時 DB / testdata / 小さい JSONL で実行し、stdout / stderr / 終了コードを確認する
- SQLite schema / migration / `backfill`: 旧形式 DB、空 DB、再実行性、DELETE 対象あり/なしを確認する
- API 仕様: N/A — 外部 API 参照なし
- ユーザー確認が必要な領域: 外部連携・実機 UI はなし
<!-- /slot -->
