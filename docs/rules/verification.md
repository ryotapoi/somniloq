# 検証ルール

変更時の build・test・実行確認と必須 gate の正本。変更内容に対応する条件付き確認に加え、完了前に共通 gate をすべて通す。

## 全変更に共通する gate

CI は `go.mod` が指定する Go version を使い、変更種別にかかわらず次を実行する。ローカルでも同じコマンドで成功を確認する。

```bash
unformatted="$(find . -name '*.go' -type f -print0 | xargs -0 gofmt -l)"
test -z "$unformatted" || { echo "$unformatted"; exit 1; }

go test -count=1 ./...
go vet ./...
go build -o bin/somniloq ./cmd/somniloq

mdhop build --vault llm-wiki
mdhop diagnose --vault llm-wiki --fields basename_conflicts,asset_basename_conflicts,phantoms,anchors --format json
```

`mdhop diagnose` は対象 field に問題がないことまで確認する。限定した package のテストは実装中の確認には使えるが、全体テストの代わりにはしない。

## 変更条件ごとの追加確認

| 変更条件 | 必須確認 |
|---|---|
| `.go` ファイル | `goimports` で整形と import 整理を行い、共通 gate の format check が空であることを確認する。 |
| CLI のフラグ、入出力、表示形式、確認プロンプト、終了コード | build した `bin/somniloq` を一時 DB と小さい JSONL または対象 testdata で実行し、成功系・失敗系の stdout、stderr、終了コードを確認する。 |
| JSONL のパース、フィルタ、正規化、import の保存境界 | 影響する Claude Code / Codex の入力例で取り込みを確認する。既存 DB と新規取り込みの意味がずれないか、`backfill` や `--full` による再取り込みの案内が必要かも確認する。 |
| SQLite schema、migration、`backfill`、DELETE を伴う処理 | 旧形式 DB と空 DB、再実行性、DELETE 対象あり・なしを確認する。確認を伴う処理では TTY での承認・拒否、`--yes`、非対話時の拒否について、データ、stdout、stderr、終了コードを確認する。 |

## 制約と例外

- Codex の PostToolUse hook は `.go` 編集後に `goimports -w` を試みるが、`jq` や `goimports` の未導入・実行失敗時も成功終了する。hook の実行を format gate の代わりにしない。
- テスト結果の cache を避けて CI と条件をそろえるため、全体テストには `-count=1` を付ける。
- permission error のテストには root 実行時に skip されるものがある。権限処理を変更した場合は非 root 環境で該当ケースを確認する。
- repository path と `backfill` の一部テストは `git` コマンドを実行する。`git` を利用できない環境での失敗を製品の回帰と判定しない。
- 外部 API、実機 UI、外部サービス連携に固有の検証 gate はない。
