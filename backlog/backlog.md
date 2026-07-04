# Backlog

## v0.7.0

2026-07-03 Knowledge 側のスキル改修（/jot /retro /rem 等）との検討で決めた機能追加に、同日の maintenance audit 指摘のうちバグに近い 1 件を加えたもの。順序は上から。

- [x] codex `ScanFiles` の walkErr 握り潰しを claudecode 側と対称にする: `internal/ingest/codex/adapter.go:53-57` が `walkErr != nil` を無条件に `nil, nil` へ変換しており、rootDir がパーミッションエラー等（ErrNotExist 以外）で読めない時にユーザーには「0 件」としか見えない。`errors.Is(walkErr, os.ErrNotExist)` を確認し、不一致ならエラーとして返す
- [x] project alias の表示正規化: sessions 一覧・show ヘッダ・projects 集計・search 結果で、旧 project 名を config の projectAliases の canonical 名で表示する。フィルタ側の展開（cmd/somniloq/config.go の expandProject、filter.go 経由）は v0.6.0 で実装済みで、出力側は生の名前のまま。まず表示に project 名が出る箇所を洗い出し、canonical 表示に統一する。出力は canonical 名のみとし、元の名前のフィールド追加はしない（旧名対応表の完全撤去が目的で、旧名を出すと読む側が再び対応を意識するため。必要になれば JSON へのフィールド追加は後方互換で足せる。2026-07-04 判断）。Knowledge の daily-guide にある旧名対応表（knowledgebase→knowledge 等）を完全に消すための残り半分
- [x] sessions にスキップ判定用の列を追加する: 「非コマンド user turn 数」と「最初の非コマンド入力の先頭 1 行」を列に出す。コマンド判定は組み込みの slash command 検出（`/` 始まり、Claude Code 用）に加え、config に `commandPatterns`（正規表現リスト）を持たせてマッチする user turn もコマンド扱いにする（Codex には slash command がなく「日報生成」のような定型起動文で叩くため、それを除外するための口）。user turn の母集団は outline の採番と同一の定義（sidechain 除外含む）を使い、「何を user turn と数えるか」の知識を 2 箇所に作らない（2026-07-04 判断）。目的は、/jot が一覧だけで定型セッション（/chk, /briefing 等のみ）を show せずに捨てられるようにすること。スキップの最終判断は CLI ではなく一覧を読む側が行う
- [x] 論理日境界（day boundary）を追加する: config に `dayBoundary`（例 "04:00"）を持ち、`--day-boundary` フラグで上書きできるようにする。変換はクエリ時計算とし、import 時に焼き込まない（境界変更で再 import が要らないように。timestamp は生のまま）。効果は 2 点: (a) sessions / search の `--since` / `--until` の日付解釈が境界起点になる（`--since 6/28` = 6/28 04:00 から）、(b) sessions に論理日の列を追加する（`--group-by day` は採らない。列なら既存の TSV / JSON 形式にそのまま乗り、グループ化は読む側でできる。2026-07-04 判断）。振り分けはセッション単位で ended_at（なければ started_at）を使い、セッションを途中で割らない。Knowledge の jot / daily-guide に散っている「04:00〜翌 04:00 を 1 日とする」ルールの移設先
- [x] outline に turn ごとのサイズ列を追加する: 各 turn の本文合計サイズ（バイト）を出す。sessions の BodySize と同じ UTF-8 byte 方式で、既存の GetMessages 結果から len(content) を合算する（スキーマ変更・import 変更なし）。長いセッションで「どの turn が重いか＝どこで何かが起きたか」の地図精度を上げ、show --turn の範囲選びを良くする
- [x] search 結果に turn 番号を含める: outline / show --turn と同一の採番で turn 番号を返し、検索ヒットからそのまま `show --turn <N..M>` に繋げられるようにする（source 間で同じ `session_id` がある場合は show の既存の曖昧エラーに従う）
- [x] ヘルプ充実と examples スキルの薄型化を行う: 各サブコマンドの `--help` に「フラグの意味・出力列の定義・実例 2〜3 個」を載せ、LLM が `--help` だけで使い方を把握できる水準にする。トップレベルの `--help` は現在の短さを維持する（全部盛りにしない）。コマンド横断のイディオム（outline → show --turn で必要範囲だけ読む等）も help 側に置く。あわせて `examples/skills/somniloq/SKILL.md` を「存在の告知・いつ使うか・--help への誘導」中心に薄くし、CLI 構文の重複記述を減らす（スキルと実装のドリフト防止）

## v0.8.0

2026-07-03 maintenance audit（deep pass、thermo-nuclear 基準）の指摘対応のうち、db.go 分割を核とした構造整理。順序は上から。

- [ ] `internal/core/db.go`（672 行）を責務別に 4 ファイルへ分割する: `db.go`（DB 構造体 + OpenDB/Close/Begin + execer）/ `db_schema.go`（ensure* マイグレーション + schema 定数）/ `db_write.go`（importTx + upsert/insert/update 群。`ingest/claudecode` への import をここに閉じる）/ `db_query.go`（SessionRow 等の型 + List/Get/Search 系 + scan ヘルパー）。同一 package 内のファイル再配置のみでサブパッケージ化はしない（`*DB` レシーバと非公開フィールドを共有するため）。テストは既に db_migration/db_write/db_query/db_search_test に分かれており、実装だけが 1 本に残っている状態の解消。公開 API 変更なし、既存テストがそのまま regression check になる
- [ ] `backfill.go` の `db.db` 非公開フィールド直接アクセスを API 経由に寄せる: `db.go` が用意した `execer` 抽象を迂回して内部レイアウトに依存しており（backfill.go:59,62,70,185,214）、DB 構造体の変更が backfill.go 広範囲に波及する。db.go 分割と同時か直後に実施
- [ ] スキーマ定義の二重記述に守りのテストを足す: `db.go` の `schema` 定数と `backfill.go` の `migrateToV04` 内 CREATE TABLE が同一カラム構成を独立に手書きしており、カラム追加時に片方だけ更新すると新規 DB と v0.3 アップグレード経由 DB でスキーマが分岐する（コンパイルもテストも通るまま）。migration SQL は凍結された歴史なので共通化はせず、「migrateToV04 実行後の DB と schema 定数から作った DB の実スキーマが一致する」ことを assert するテストを 1 本追加する
- [ ] マイグレーション・インポート異常系のテストを追加する: 優先度高 = `OpenDB` 異常系と `ensureSessions*` のレースリチェック分岐（全コマンドの入口、壊れると静かな不整合）。中 = `migrateToV04` の PRAGMA 復元失敗パス（不可逆操作）、`Backfill` の Unresolved 分岐、`importWithAdapter` のファイル縮小分岐（壊れると二重インポートかデータ欠損）。監査時に低とされた `importCmd` の flag 組み合わせと `resolveSessionByID` の複数マッチ分岐は、壊れてもすぐ見える failure のため対象外（2026-07-03 判断）。adapter 群は統合テスト実測 75-100% のため追加不要
- [ ] `docs/rules/architecture.md` に core → ingest/claudecode 依存の例外を明記する: 依存方向図が単純な線形のみで、`db.go` の `claudecode.SessionMetaWriter` 実装用 import（ADR 0008 で採用済み）を次の読み手が「違反」と誤認するか、逆に安易な source 直 import を正当化しかねない。依存方向節に例外 1 行 + ADR 0008 参照を足す

## v0.9.0

2026-07-03 maintenance audit の指摘対応の残り（互いに独立した軽い整理）。順序は上から。

- [ ] クロスコマンドヘルパーの配置を集約先に揃える: `sanitizeTSV` が `sessions.go` にありながら search/outline からも呼ばれ、`resolveSessionByID` / `writeAmbiguousSessionError` も `show.go` にありながら outline から呼ばれる。「ファイル名 = 担当コマンド」の前提が崩れているので、format.go か共通ヘルパーファイルへ移動する
- [ ] CLI の source 文字列を `importSourceSpecs` テーブルから導出する: 新 source 追加時に `Valid()` は通るのに `--help` とエラーメッセージ（cmd/somniloq/import.go の 2 箇所）だけ古いままになる結合の解消。テーブルから文字列リストを生成する関数を core に置き、CLI 側を置換する
- [ ] adapter の永続化 3 ステップを ingest 共通ヘルパーへ切り出す: claudecode/codex の `HandleLine` 末尾（UpsertSession → 空コンテンツチェック → InsertMessage）が変数名以外同一の約 10 行。「空コンテンツはセッションのみ登録して本文を書かない」という永続化ポリシーは source 非依存の同一知識であり、record の解釈（source 固有）とは意味境界が別なので、`ingest.PersistMessage` のような共通ヘルパーに集約する（2026-07-03 設計判断済み）。永続化を adapter の外へ出すことは process.go の FileHandler 責務コメント（record interpretation に限定）とも整合する
