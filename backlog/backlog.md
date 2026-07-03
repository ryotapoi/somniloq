# Backlog

## v0.7.0

2026-07-03 Knowledge 側のスキル改修（/jot /retro /rem 等）との検討で決めた機能追加。順序は上から。

- [ ] project alias の表示正規化: sessions 一覧・show ヘッダ・projects 集計・search 結果で、旧 project 名を config の projectAliases の canonical 名で表示する。フィルタ側の展開（cmd/somniloq/config.go の expandProject、filter.go 経由）は v0.6.0 で実装済みで、出力側は生の名前のまま。まず表示に project 名が出る箇所を洗い出し、canonical 表示に統一する。元の名前を残すか（JSON フィールド追加等）は設計時に判断。Knowledge の daily-guide にある旧名対応表（knowledgebase→knowledge 等）を完全に消すための残り半分
- [ ] sessions にスキップ判定用の列を追加する: 「非コマンド user turn 数」と「最初の非コマンド入力の先頭 1 行」を列に出す。コマンド判定は組み込みの slash command 検出（`/` 始まり、Claude Code 用）に加え、config に `commandPatterns`（正規表現リスト）を持たせてマッチする user turn もコマンド扱いにする（Codex には slash command がなく「日報生成」のような定型起動文で叩くため、それを除外するための口）。目的は、/jot が一覧だけで定型セッション（/chk, /briefing 等のみ）を show せずに捨てられるようにすること。スキップの最終判断は CLI ではなく一覧を読む側が行う
- [ ] 論理日境界（day boundary）を追加する: config に `dayBoundary`（例 "04:00"）を持ち、`--day-boundary` フラグで上書きできるようにする。変換はクエリ時計算とし、import 時に焼き込まない（境界変更で再 import が要らないように。timestamp は生のまま）。効果は 2 点: (a) sessions / search の `--since` / `--until` の日付解釈が境界起点になる（`--since 6/28` = 6/28 04:00 から）、(b) sessions に論理日の列（または `--group-by day`）を追加する。振り分けはセッション単位で ended_at（なければ started_at）を使い、セッションを途中で割らない。Knowledge の jot / daily-guide に散っている「04:00〜翌 04:00 を 1 日とする」ルールの移設先
- [ ] outline に turn ごとのサイズ列を追加する: 各 turn の本文合計サイズ（バイト）を出す。sessions の BodySize と同じ方式でクエリ時に SUM(LENGTH(content)) を計算する（スキーマ変更・import 変更なし）。長いセッションで「どの turn が重いか＝どこで何かが起きたか」の地図精度を上げ、show --turn の範囲選びを良くする
- [ ] search 結果に turn 番号を含める: outline / show --turn と同一の採番で turn 番号を返し、検索ヒットからそのまま `show --turn <N..M>` に繋げられるようにする
- [ ] ヘルプ充実と examples スキルの薄型化を行う: 各サブコマンドの `--help` に「フラグの意味・出力列の定義・実例 2〜3 個」を載せ、LLM が `--help` だけで使い方を把握できる水準にする。トップレベルの `--help` は現在の短さを維持する（全部盛りにしない）。コマンド横断のイディオム（outline → show --turn で必要範囲だけ読む等）も help 側に置く。あわせて `examples/skills/somniloq/SKILL.md` を「存在の告知・いつ使うか・--help への誘導」中心に薄くし、CLI 構文の重複記述を減らす（スキルと実装のドリフト防止）

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
