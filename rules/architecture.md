# アーキテクチャ

## モジュール構成

```
cmd/somniloq/       CLI エントリーポイント。フラグ解析・出力フォーマット・サブコマンドルーティング
internal/core/      ビジネスロジック。SQLite 操作・インポート制御・クエリ
internal/ingest/    source 別 JSONL adapter。ファイル走査・JSONL パース・共通正規化型
```

## 依存方向

```
cmd/somniloq → internal/core → internal/ingest/...
```

- `internal/core` は `cmd/somniloq` に依存しない
- `internal/ingest` は `cmd/somniloq` に依存しない
- `internal/ingest` は `internal/core` に依存しない。SQLite 書き込みは `internal/core` が実装する interface 越しに呼ぶ
- `internal/core` は外部ライブラリとして `modernc.org/sqlite` のみ使用
- `cmd/somniloq` は stdlib `flag` + `go-isatty` を使用（外部 CLI フレームワーク不使用）

## 責務の境界

| モジュール | 責務 | やらないこと |
|-----------|------|-------------|
| `cmd/somniloq` | CLI 入出力、フラグ解析、出力フォーマット（text/Markdown）、エラーメッセージ表示 | DB 操作、JSONL パース |
| `internal/core` | DB スキーマ管理、インポート制御、クエリ、adapter から呼ばれる SQLite 書き込み実装 | CLI フラグ解析、出力フォーマット、`os.Exit`、source 固有 JSONL パース |
| `internal/ingest` | 共通正規化型、adapter interface、source 固有のファイル走査・JSONL パース・正規化 | CLI フラグ解析、出力フォーマット、SQLite SQL、`os.Exit` |

## 新モジュールを切る判断基準

source 別 JSONL 形式の差異は `internal/ingest/<source>/` に閉じ込める。DB スキーマ・SQL・検索は `internal/core` に残す。

追加分割を検討する目安:

- `internal/core` / `internal/ingest` のどちらかで責務の異なるグループが明確になった場合
- 外部から `internal/core` または `internal/ingest` の一部だけを使いたいケースが発生した場合
