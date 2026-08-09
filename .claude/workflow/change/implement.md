# Implement Workflow

正本は `~/.config/agents/workflow/change/implement.md`。これを Read し、以下のプロジェクト固有の tooling slot を埋めて実行する。

## Tooling

<!-- slot: ビルド・テスト・実行のコマンドと使うツールを書く（例: XcodeBuildMCP の build_macos / test_macos と「Bash で xcodebuild を直接叩かない」、go build ./... / go test ./...、./gradlew verifyAll、CLI なら bin/<tool>）。API 仕様の一次情報確認手段も書く（外部 API が無ければ「N/A — 外部 API 参照なし」と明記する）。 -->
- ビルド: `go build -o bin/somniloq ./cmd/somniloq`
- テスト: `go test ./...`
- 静的チェック: `go vet ./...`
- フォーマット: `.go` 編集後に PostToolUse hook（`.claude/hooks/go-format.sh` が `goimports -w` を実行）で自動整形されるため手動不要
- CLI 実行: `bin/somniloq <command>` を一時 DB / testdata / 小さい JSONL で実行し、stdout / stderr / 終了コードを確認する
- 依存方向の禁止: `internal/core` に CLI 入出力や `os.Exit` を持ち込まない。`cmd/somniloq` に DB 操作や JSONL パースを持ち込まない（`cmd/somniloq -> internal/core` の一方向）
- API 仕様: N/A — 外部 API 参照なし
<!-- /slot -->
