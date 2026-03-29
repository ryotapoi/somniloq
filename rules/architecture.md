# アーキテクチャ

## モジュール構成

```
cmd/somniloq/       CLI エントリーポイント。フラグ解析・出力フォーマット・サブコマンドルーティング
internal/core/   ビジネスロジック。JSONL パース・SQLite 操作・インポート・クエリ
```

## 依存方向

```
cmd/somniloq → internal/core
```

- `internal/core` は `cmd/somniloq` に依存しない
- `internal/core` は外部ライブラリとして `modernc.org/sqlite` のみ使用
- `cmd/somniloq` は stdlib `flag` + `go-isatty` を使用（外部 CLI フレームワーク不使用）

## 責務の境界

| モジュール | 責務 | やらないこと |
|-----------|------|-------------|
| `cmd/somniloq` | CLI 入出力、フラグ解析、出力フォーマット（text/Markdown）、エラーメッセージ表示 | DB 操作、JSONL パース |
| `internal/core` | JSONL パース、DB スキーマ管理、インポート、クエリ | CLI フラグ解析、出力フォーマット、`os.Exit` |

## 新モジュールを切る判断基準

現時点では `internal/core` が全ビジネスロジックを担う。以下の場合に分割を検討する:

- `internal/core` のファイル数が 20 を超え、責務の異なるグループが明確になった場合
- 外部から `internal/core` の一部だけを使いたいケースが発生した場合
