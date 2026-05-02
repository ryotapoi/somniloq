# 技術的知見

## タイムスタンプの精度

Claude Code の JSONL タイムスタンプはミリ秒付き RFC3339（例: `2026-03-28T14:10:45.977Z`）。`started_at` / `ended_at` はこの値がそのまま保存される。

文字列比較で時刻フィルタを行う場合、比較対象もミリ秒付きにする必要がある。`time.RFC3339`（秒精度、末尾 `Z`）で生成すると、`.` (0x2E) < `Z` (0x5A) の関係で同秒内のミリ秒付きデータが誤って除外される。

対処: `"2006-01-02T15:04:05.000Z"` フォーマットを使い、常に3桁のミリ秒を出力する。

## modernc.org/sqlite の `:memory:` は接続ごとに別 DB

`:memory:` DSN は `database/sql` の接続プールの各物理接続ごとに独立したインメモリ DB を作る。同じ `*sql.DB` でもクエリごとに別の接続に割り振られると、先の PRAGMA で見ていたテーブルが次の ALTER では存在しない、といった事故になる。

対処: `OpenDB` で `db.SetMaxOpenConns(1)` を設定し、物理接続を 1 本に固定する。ファイル DB でも SQLite の書き込みロック特性上直列化されるので実害はなく、本プロジェクトの CLI 用途では一律 1 本で十分。

## modernc.org/sqlite の `RowsAffected` / `LastInsertId` は常に nil エラー

`modernc.org/sqlite` の `sql.Result.RowsAffected()` / `LastInsertId()` は常にエラー `nil` を返す（実装が pure Go の SQLite で、ドライバ層で値を確定できる）。エラーチェックを書いても本ドライバでは絶対に発火しないため、テストでエラーパスを再現できない。

対処: エラーチェックは将来のドライバ切替時の防御として残すが、`fmt.Errorf("...: %w", err)` でラップする際に「modernc では常に nil」と短いコメントを添えると意図が伝わる。ドライバ依存の挙動なので、別ドライバ（mattn/go-sqlite3 など）に切り替えると挙動が変わる可能性がある。

## スキーママイグレーションは PRAGMA check-first を主経路にする

SQLite には `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` が無い。`duplicate column name` エラーを文字列マッチで吸収する方法はドライバのエラーフォーマット変更に脆い。

対処: `PRAGMA table_info(<table>)` で列の有無を先に確認し、無ければ ALTER を打つ。並行起動等で ALTER が失敗した場合は、再度 `PRAGMA table_info` で現状を確認し、列が既に存在すれば成功扱い、そうでなければ元の ALTER エラーを返す。状態ベースの判定で、ドライバのエラー文字列に依存しない。
