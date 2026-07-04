# Backlog

## v0.7.1

2026-07-03 maintenance audit（deep pass、thermo-nuclear 基準）の指摘対応のうち、db.go 分割を核とした構造整理。順序は上から。

- [x] `internal/core/db.go`（672 行）を責務別に 4 ファイルへ分割する: `db.go`（DB 構造体 + OpenDB/Close/Begin + execer）/ `db_schema.go`（ensure* マイグレーション + schema 定数）/ `db_write.go`（importTx + upsert/insert/update 群。`ingest/claudecode` への import をここに閉じる）/ `db_query.go`（SessionRow 等の型 + List/Get/Search 系 + scan ヘルパー）。同一 package 内のファイル再配置のみでサブパッケージ化はしない（`*DB` レシーバと非公開フィールドを共有するため）。テストは既に db_migration/db_write/db_query/db_search_test に分かれており、実装だけが 1 本に残っている状態の解消。公開 API 変更なし、既存テストがそのまま regression check になる
- [x] `backfill.go` の `db.db` 非公開フィールド直接アクセスを API 経由に寄せる: `db.go` が用意した `execer` 抽象を迂回して内部レイアウトに依存しており（backfill.go:59,62,70,185,214）、DB 構造体の変更が backfill.go 広範囲に波及する。db.go 分割と同時か直後に実施
- [ ] スキーマ定義の二重記述に守りのテストを足す: `db.go` の `schema` 定数と `backfill.go` の `migrateToV04` 内 CREATE TABLE が同一カラム構成を独立に手書きしており、カラム追加時に片方だけ更新すると新規 DB と v0.3 アップグレード経由 DB でスキーマが分岐する（コンパイルもテストも通るまま）。migration SQL は凍結された歴史なので共通化はせず、「migrateToV04 実行後の DB と schema 定数から作った DB の実スキーマが一致する」ことを assert するテストを 1 本追加する
- [ ] マイグレーション・インポート異常系のテストを追加する: 優先度高 = `OpenDB` 異常系と `ensureSessions*` のレースリチェック分岐（全コマンドの入口、壊れると静かな不整合）。中 = `migrateToV04` の PRAGMA 復元失敗パス（不可逆操作）、`Backfill` の Unresolved 分岐、`importWithAdapter` のファイル縮小分岐（壊れると二重インポートかデータ欠損）。監査時に低とされた `importCmd` の flag 組み合わせと `resolveSessionByID` の複数マッチ分岐は、壊れてもすぐ見える failure のため対象外（2026-07-03 判断）。adapter 群は統合テスト実測 75-100% のため追加不要
- [ ] `docs/rules/architecture.md` に core → ingest/claudecode 依存の例外を明記する: 依存方向図が単純な線形のみで、`db.go` の `claudecode.SessionMetaWriter` 実装用 import（ADR 0008 で採用済み）を次の読み手が「違反」と誤認するか、逆に安易な source 直 import を正当化しかねない。依存方向節に例外 1 行 + ADR 0008 参照を足す

## v0.7.2

2026-07-03 maintenance audit の指摘対応の残り（互いに独立した軽い整理）。順序は上から。

- [ ] クロスコマンドヘルパーの配置を集約先に揃える: `sanitizeTSV` が `sessions.go` にありながら search/outline からも呼ばれ、`resolveSessionByID` / `writeAmbiguousSessionError` も `show.go` にありながら outline から呼ばれる。「ファイル名 = 担当コマンド」の前提が崩れているので、format.go か共通ヘルパーファイルへ移動する
- [ ] CLI の source 文字列を `importSourceSpecs` テーブルから導出する: 新 source 追加時に `Valid()` は通るのに `--help` とエラーメッセージ（cmd/somniloq/import.go の 2 箇所）だけ古いままになる結合の解消。テーブルから文字列リストを生成する関数を core に置き、CLI 側を置換する
- [ ] adapter の永続化 3 ステップを ingest 共通ヘルパーへ切り出す: claudecode/codex の `HandleLine` 末尾（UpsertSession → 空コンテンツチェック → InsertMessage）が変数名以外同一の約 10 行。「空コンテンツはセッションのみ登録して本文を書かない」という永続化ポリシーは source 非依存の同一知識であり、record の解釈（source 固有）とは意味境界が別なので、`ingest.PersistMessage` のような共通ヘルパーに集約する（2026-07-03 設計判断済み）。永続化を adapter の外へ出すことは process.go の FileHandler 責務コメント（record interpretation に限定）とも整合する
