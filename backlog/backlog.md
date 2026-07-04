# Backlog

## v0.7.2

2026-07-03 maintenance audit の指摘対応の残り（互いに独立した軽い整理）。順序は上から。

- [x] クロスコマンドヘルパーの配置を集約先に揃える: `sanitizeTSV` が `sessions.go` にありながら search/outline からも呼ばれ、`resolveSessionByID` / `writeAmbiguousSessionError` も `show.go` にありながら outline から呼ばれる。「ファイル名 = 担当コマンド」の前提が崩れているので、format.go か共通ヘルパーファイルへ移動する
- [ ] CLI の source 文字列を `importSourceSpecs` テーブルから導出する: 新 source 追加時に `Valid()` は通るのに `--help` とエラーメッセージ（cmd/somniloq/import.go の 2 箇所）だけ古いままになる結合の解消。テーブルから文字列リストを生成する関数を core に置き、CLI 側を置換する
- [ ] adapter の永続化 3 ステップを ingest 共通ヘルパーへ切り出す: claudecode/codex の `HandleLine` 末尾（UpsertSession → 空コンテンツチェック → InsertMessage）が変数名以外同一の約 10 行。「空コンテンツはセッションのみ登録して本文を書かない」という永続化ポリシーは source 非依存の同一知識であり、record の解釈（source 固有）とは意味境界が別なので、`ingest.PersistMessage` のような共通ヘルパーに集約する（2026-07-03 設計判断済み）。永続化を adapter の外へ出すことは process.go の FileHandler 責務コメント（record interpretation に限定）とも整合する
