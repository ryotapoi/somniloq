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

go install github.com/google/go-licenses/v2@v2.0.1
python3 scripts/update-third-party-notices.py --check

```

限定した package のテストは実装中の確認には使えるが、全体テストの代わりにはしない。

通知の生成方法と配布時の扱いは [release](release.md) に従う。

## 変更条件ごとの追加確認

| 変更条件 | 必須確認 |
|---|---|
| `.go` ファイル | `goimports` で整形と import 整理を行い、共通 gate の format check が空であることを確認する。 |
| CLI のフラグ、入出力、表示形式、確認プロンプト、終了コード | build した `bin/somniloq` を一時 DB と小さい JSONL または対象 testdata で実行し、成功系・失敗系の stdout、stderr、終了コードを確認する。 |
| JSONL のパース、フィルタ、正規化、import の保存境界 | 影響する Claude Code / Codex / Cursor Agent の入力例で取り込みを確認する。Cursor Agent を含む共通 ingest の変更では、[JSONL 形式](jsonl-format.md) の Cursor Agent 節に従い、未知 timestamp を補完しないこと、path 由来の session / message identity、物理行を保持する差分取り込み・再処理で重複や順序ずれを生まないことも確認する。既存 DB と新規取り込みの意味がずれないか、`--full` による再取り込みの案内が必要かも確認する。 |
| SQLite schema、migration、DELETE を伴う処理 | 一般 schema 管理では対象の旧形式 DB と空 DB、再実行性を確認する。DELETE を伴う処理では対象あり・なしを確認する。専用 migrate の方針は [ADR 0024](../decisions/0024-owner-history-migration.md)、検証は [移行 fixture](../internal/ingest/testdata/v0.14.0-migration/README.md) で初回コピー・同一 snapshot 再実行・拒否・行単位置換・失敗時保持を確認し、移行元 bytes と sidecar 状態の不変も確認する。確認を伴う処理では TTY での承認・拒否、`--yes`、非対話時の拒否について、データ、stdout、stderr、終了コードを確認する。 |

## SQL と集約のレビュー

`MAX(<TEXT カラム>)` を含む SQL 変更では順序の意味を確認する。集約 query では GROUP BY key と出力値の変換が一致するか確認する。import のフィルタ・スキップ・保存境界を変えるときは既存 DB と新規取り込みの意味の差を確認し、必要なら `--full` の案内を更新する。CLI のフラグ・コマンド・表示を変えたら usage、help、README 両言語を同じ変更で同期する。

## 制約と例外

- Codex の PostToolUse hook は `.go` 編集後に `goimports -w` を試みるが、`jq` や `goimports` の未導入・実行失敗時も成功終了する。hook の実行を format gate の代わりにしない。
- テスト結果の cache を避けて CI と条件をそろえるため、全体テストには `-count=1` を付ける。
- permission error のテストには root 実行時に skip されるものがある。権限処理を変更した場合は非 root 環境で該当ケースを確認する。
- repository path の一部テストは `git` コマンドを実行する。`git` を利用できない環境での失敗を製品の回帰と判定しない。
- 外部 API、実機 UI、外部サービス連携に固有の検証 gate はない。
