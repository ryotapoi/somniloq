# Backlog

## Maintenance（2026-07 監査）

2026-07-03 maintenance audit（deep pass、thermo-nuclear 基準）の指摘対応。順序は上から（優先度順）。

- [ ] `internal/core/db.go`（672 行）を責務別に 4 ファイルへ分割する: `db.go`（DB 構造体 + OpenDB/Close/Begin + execer）/ `db_schema.go`（ensure* マイグレーション + schema 定数）/ `db_write.go`（importTx + upsert/insert/update 群。`ingest/claudecode` への import をここに閉じる）/ `db_query.go`（SessionRow 等の型 + List/Get/Search 系 + scan ヘルパー）。同一 package 内のファイル再配置のみでサブパッケージ化はしない（`*DB` レシーバと非公開フィールドを共有するため）。テストは既に db_migration/db_write/db_query/db_search_test に分かれており、実装だけが 1 本に残っている状態の解消。公開 API 変更なし、既存テストがそのまま regression check になる
- [ ] スキーマ定義の二重記述に守りのテストを足す: `db.go` の `schema` 定数と `backfill.go` の `migrateToV04` 内 CREATE TABLE が同一カラム構成を独立に手書きしており、カラム追加時に片方だけ更新すると新規 DB と v0.3 アップグレード経由 DB でスキーマが分岐する（コンパイルもテストも通るまま）。migration SQL は凍結された歴史なので共通化はせず、「migrateToV04 実行後の DB と schema 定数から作った DB の実スキーマが一致する」ことを assert するテストを 1 本追加する
- [ ] codex `ScanFiles` の walkErr 握り潰しを claudecode 側と対称にする: `internal/ingest/codex/adapter.go:53-57` が `walkErr != nil` を無条件に `nil, nil` へ変換しており、rootDir がパーミッションエラー等（ErrNotExist 以外）で読めない時にユーザーには「0 件」としか見えない。`errors.Is(walkErr, os.ErrNotExist)` を確認し、不一致ならエラーとして返す
- [ ] `docs/rules/architecture.md` に core → ingest/claudecode 依存の例外を明記する: 依存方向図が単純な線形のみで、`db.go` の `claudecode.SessionMetaWriter` 実装用 import（ADR 0008 で採用済み）を次の読み手が「違反」と誤認するか、逆に安易な source 直 import を正当化しかねない。依存方向節に例外 1 行 + ADR 0008 参照を足す
- [ ] マイグレーション・インポート異常系のテストを追加する: 優先度高 = `OpenDB` 異常系と `ensureSessions*` のレースリチェック分岐（全コマンドの入口、壊れると静かな不整合）。中 = `migrateToV04` の PRAGMA 復元失敗パス（不可逆操作）、`Backfill` の Unresolved 分岐、`importWithAdapter` のファイル縮小分岐（壊れると二重インポートかデータ欠損）。低 = `importCmd` の `--yes`/`--full`/isTTY 組み合わせ、`resolveSessionByID` の複数マッチ分岐。adapter 群は統合テスト実測 75-100% のため追加不要
- [ ] `backfill.go` の `db.db` 非公開フィールド直接アクセスを API 経由に寄せる: `db.go` が用意した `execer` 抽象を迂回して内部レイアウトに依存しており（backfill.go:59,62,70,185,214）、DB 構造体の変更が backfill.go 広範囲に波及する。db.go 分割と同時か直後に実施
- [ ] クロスコマンドヘルパーの配置を集約先に揃える: `sanitizeTSV` が `sessions.go` にありながら search/outline からも呼ばれ、`resolveSessionByID` / `writeAmbiguousSessionError` も `show.go` にありながら outline から呼ばれる。「ファイル名 = 担当コマンド」の前提が崩れているので、format.go か共通ヘルパーファイルへ移動する
- [ ] CLI の source 文字列を `importSourceSpecs` テーブルから導出する: 新 source 追加時に `Valid()` は通るのに `--help` とエラーメッセージ（cmd/somniloq/import.go の 2 箇所）だけ古いままになる結合の解消。テーブルから文字列リストを生成する関数を core に置き、CLI 側を置換する
- [ ] 【要設計判断】adapter の永続化 3 ステップ重複を解消するか決める: claudecode/codex の `HandleLine` 末尾（UpsertSession → 空コンテンツチェック → InsertMessage）が変数名以外同一の 9 行で、仕様変更時に片方だけ直すと source 間で挙動が分岐する。`ingest.PersistRecord` のような共通ヘルパーへの切り出しと、process.go の FileHandler 責務コメント（record interpretation に限定）との整合をどう取るかの境界設計が要る
- [ ] 【要設計判断】show の 3 モード（--summary/--turn/--tail）の二重表現を解消するか決める: 相互排他バリデーション（show.go:52-58）と getMessages クロージャ組み立て（show.go:96-119）が同じ 3 モードを別々の場所で表現しており、モード追加時に 2 箇所の連動が必要。ShowMode 型に寄せて分岐を 1 箇所に集約するか、buildMessageFetcher 抽出で済ませるかの判断が要る

## v0.6.0

2026-06-11 Knowledge 側の運用（/jot /retro /rem、LLM Wiki 連携の見据え）からの機能追加。順序は上から。

- [x] セッションのアウトライン表示を追加する: ユーザーメッセージだけを「ターン番号・時刻・先頭 1 行」で時系列に並べ、長いセッションの構造を全文 show する前に掴めるようにする（`somniloq outline <session-id>` を採用）。sidechain 除外は show と同じ扱い。Knowledge の /jot が「長セッションは全文をファイルに出して haiku subagent に時系列地図を作らせる」手順で代替している箇所の置き換え先
- [x] show の部分読みを追加する: `show <session-id> --turn 40..60` / `--tail <N>` のようにターン範囲を指定して読めるようにする。アウトラインで見つけた範囲だけ読む用途。ターン番号はアウトライン表示と同一の採番を共有すること
- [x] sessions 一覧にサイズ列を追加する: セッションの本文合計サイズ（バイト数を採用）を列に出し、show する前に「大きいセッションか」を判定できるようにする。MessageCount だけでは 1 メッセージが巨大なセッションを見分けられない
- [x] `--format json` を追加する: sessions / show / projects（アウトラインも）で JSON 出力を選べるようにする。Knowledge の list-sessions.sh が show の Markdown を awk でパースしている脆さの解消先
- [x] search コマンドを追加する: `somniloq search <query> [--since] [--until] [--project]` で、セッション ID・時刻・マッチ前後のスニペットを返す。実装は LIKE 全走査でよい（本文 42 MB の現 DB で実測 0.11 秒。FTS5 は日本語だと trigram 必須で、索引が本文の 2〜3 倍に膨らむ・3 文字未満のクエリが索引で引けない制約があるため、LIKE で困るスケールになるまで見送り）
- [x] project alias 設定を追加する: `~/.somniloq/config.json`（`--config` フラグで上書き可、JSON なので依存追加なし）に `projectAliases`（例: `"brimday": ["whisday"]`）を持ち、`--project` 指定時にグループ内のどの名前でもマッチするよう展開する。リネーム済みリポジトリの歴史を 1 つの project として引けるようにし、Knowledge の daily-guide にベタ書きされている旧名対応表の移設先にする
