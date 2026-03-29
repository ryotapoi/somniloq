# ADR 0002: Go formatter に goimports を採用

## Status

Accepted

## Context

Claude Code の PostToolUse Hook で Go ファイルの自動フォーマットを導入するにあたり、フォーマッタの選定が必要になった。Koecho（Swift プロジェクト）では「公式ツール・デフォルト設定・最小構成」の方針で swift-format を採用しており、Go でも同じ方針を踏襲する。

Go エコシステムのフォーマッタ候補:
- `gofmt`: Go 公式。コードスタイルの統一のみ
- `goimports`: gofmt の上位互換 + import 整理。`golang.org/x/tools` で Go チームメンバーが開発
- `gofumpt`: gofmt より厳格。Go チーム外の開発

リンターについても検討が必要:
- `go vet`: Go 公式。`go test` で自動実行される
- `golangci-lint`: コミュニティ製メタリンター
- `staticcheck`: コミュニティ製。高品質だが非公式

## Considered Options

- **gofmt のみ**: 最も保守的だが、未使用 import の削除や import グルーピングが行われない
- **goimports**: gofmt + import 整理。準公式（golang.org/x/tools）で事実上の標準
- **gofumpt**: より厳格なフォーマット。Go チーム外の開発で意見が強い

リンター:
- **go vet のみ（Hook に入れない）**: 公式リンターは `go test` で既に実行される。ロジック系チェックはレビュースキルに委譲
- **golangci-lint を Hook に追加**: 非公式。最小構成の方針に合わない

## Decision

We will use `goimports` as the Go formatter in the PostToolUse Hook, with default settings. We will not add any linter to the Hook.

- goimports は gofmt の完全上位互換であり、import 整理という実用的な付加価値がある
- `golang.org/x/tools` は Go チームの準公式リポジトリで、Swift における Apple 公式 swift-format と同等の位置づけ
- `go vet`（公式リンター）は `go test` で自動実行されるため Hook 追加は不要
- 非公式リンター（golangci-lint 等）は導入しない

## Consequences

- Go ファイルの Write/Edit 後に自動でフォーマット + import 整理が行われる
- goimports が PATH にない環境では Hook がサイレントに失敗する（exit 0 固定で操作は阻害しない）
- 将来 gofumpt 等に切り替えたい場合、Hook スクリプトのコマンドを差し替えるだけで対応可能
