# Backlog

## v0.5

2026-06-11 のコード・ドキュメント監査（thermo-nuclear-code-quality-review）より。変更容易性・メンテナンス性の改善。順序は上から。

- [x] cmd / ingest 層の小掃除: `runImport` → `runImportWith` の identity wrapper を削除（`cmd/somniloq/main.go`、テスト含め他に呼び出し元なし）。`RepoResolver` 型の重複定義（`internal/ingest/claudecode/adapter.go` と `internal/ingest/codex/adapter.go` に同一定義）を `internal/ingest` に一本化。`core.ParsedMessage`（`internal/core/types.go`）のエイリアス名を `NormalizedMessage` に揃え、同一型に 2 つの名前がある状態を解消
- [x] cmd のサブコマンド実装形式を `backfillCmd` 型に統一する: `runImport` / `runSessions` / `runShow` / `runProjects` は `fmt.Fprintf(os.Stderr, ...)` + `os.Exit(1)` の直書きが散在しテスト不能。`backfillCmd`（exit code と error を返し `os.Exit` しない形式）に揃え、`os.Exit` は `main` に集約する。合わせて `main.go`（465 行、全サブコマンド同居）をサブコマンド毎のファイルに分割する
- [x] backfill の migration 呼び出し経路を一本化する: `BackfillResult.MigratedSessions` 等は CLI 経由だと常に 0 になる「直接呼び出し時のみ有効」な罠フィールド（`internal/core/backfill.go` 冒頭コメント参照）。`MigrateToV04IfNeeded` を `Backfill` 内部から外し、呼び出し側で常に preflight する形（`CountOrphanSessions` と同じ precondition 方式）に統一して、罠フィールドと補足コメント群を削除する
- [x] `internal/core/db.go` の session row SELECT / Scan の重複を集約する: 同じ列リスト + Scan が `ListSessions` / `GetSession` / `LookupSessionsByID` の 3 箇所にあり、sessions へのカラム追加時に 3 箇所の同期変更が要る。`scanMessages` と同様の helper に寄せる。`Since` / `Until` 条件の組み立ても `ListSessions` / `ListProjects` で重複しているので同時に集約する
- [x] sidechain 除外フィルタを 1 層に寄せる: `GetSummaryMessages` は SQL（`is_sidechain = 0`）、show 全文表示は表示層（`cmd/somniloq/format.go` の `formatSession` 内 skip）と、同じ仕様が 2 層に分散している。SQL 側に寄せる
- [ ] import source の列挙散在を集約する: 新 source 追加時に `ImportSource` 定数 / `valid()` / `parseImportSource`（cmd 側で同じ列挙チェックが二重）/ `core.Import` の switch（all と単独で同じ adapter 呼び出しが重複）/ `main.go` のデフォルトディレクトリ引数受け渡し、と触る箇所が散っている。source → (adapter 生成, デフォルトディレクトリ, CLI 表記) の対応表 1 箇所に集約して switch を畳む
- [ ] adapter `ProcessFile` の骨格重複の扱いを設計判断して実施する: open / seek / offset 追跡 / `ReadBytes` ループ / `hasBody` / `import_state` upsert / commit が claudecode と codex でほぼ同型に重複し、codex は `scanPrefix` に 3 つ目の同型ループがある。3rd source 追加前に「共通ループの抽出」か「adapter 責務を行→record 変換に縮小」かを design-decision / module-boundary で決める。`ingest.ImportTransaction` に Claude Code 固有の `UpdateSessionTitle` / `UpdateSessionAgentName` が載っている（source が増えると union interface 化する）件も同時に扱う
- [ ] parse 失敗行の silent skip を可視化するか仕様として明文化する（仕様判断）: 両 adapter とも `ParseRecord` / Normalize 失敗行を `continue` で黙殺し、件数も残らない（壊れ JSON と未知 type の区別なし）。「なぜこのメッセージが DB にないか」を追えないため、`ImportResult` に skip 行カウントを足すか、黙殺を仕様として `rules/scope.md` に明記するかを決める
- [ ] import のディレクトリ走査エラーをファイル単位エラーと同じ非致命扱いにする: 現状、ログディレクトリ配下に読めないディレクトリが 1 つあると import 全体が即エラー終了する（`--source all` なら他 source 分も含め 1 ファイルも取り込まれない）。ファイル単位の失敗は `FilesFailed` カウント + 続行なので、走査の失敗も同様にスキップして `ImportResult.Errors` に記録し、exit code に反映して続行する（両 source の `ScanFiles` が対象）。`ScanFiles` の戻り値契約（部分結果 + 非致命エラー）が変わるため、adapter 骨格タスクで決めた interface 設計に合わせること
- [ ] `internal/core/db_test.go`（1745 行）を機能別に分割する: migration 系 / write 系 / query 系で分け、仕様索引としての探索性を回復する
- [ ] `decisions/` の連番桁数を統一する: `001-language-go.md` のみ 3 桁で他は 4 桁。リネームと参照箇所の更新
- [ ] `rules/scope.md`「Known limitations / 移行期限定」の削除条件を具体化する: 「データ補正完了が一般化したら削除する」とあるが判定条件がなく恒久残置になりかけている。条件を決めるか、いま削除可否を判断する

## v0.6

2026-06-11 Knowledge 側の運用（/jot /retro /rem、LLM Wiki 連携の見据え）からの機能追加。順序は上から。

- [ ] セッションのアウトライン表示を追加する: ユーザーメッセージだけを「ターン番号・時刻・先頭 1 行」で時系列に並べ、長いセッションの構造を全文 show する前に掴めるようにする（`somniloq outline <session-id>` か `show --toc` かは実装時に判断）。sidechain 除外は show と同じ扱い。Knowledge の /jot が「長セッションは全文をファイルに出して haiku subagent に時系列地図を作らせる」手順で代替している箇所の置き換え先
- [ ] show の部分読みを追加する: `show <session-id> --turn 40..60` / `--tail <N>` のようにターン範囲を指定して読めるようにする。アウトラインで見つけた範囲だけ読む用途。ターン番号はアウトライン表示と同一の採番を共有すること
- [ ] sessions 一覧にサイズ列を追加する: セッションの本文合計サイズ（文字数 or バイト数）を列に出し、show する前に「大きいセッションか」を判定できるようにする。MessageCount だけでは 1 メッセージが巨大なセッションを見分けられない
- [ ] `--format json` を追加する: sessions / show / projects（アウトラインも）で JSON 出力を選べるようにする。Knowledge の list-sessions.sh が show の Markdown を awk でパースしている脆さの解消先
- [ ] search コマンドを追加する: `somniloq search <query> [--since] [--until] [--project]` で、セッション ID・時刻・マッチ前後のスニペットを返す。実装は LIKE 全走査でよい（本文 42 MB の現 DB で実測 0.11 秒。FTS5 は日本語だと trigram 必須で、索引が本文の 2〜3 倍に膨らむ・3 文字未満のクエリが索引で引けない制約があるため、LIKE で困るスケールになるまで見送り）
- [ ] project alias 設定を追加する: `~/.somniloq/config.json`（`--config` フラグで上書き可、JSON なので依存追加なし）に `projectAliases`（例: `"brimday": ["whisday"]`）を持ち、`--project` 指定時にグループ内のどの名前でもマッチするよう展開する。リネーム済みリポジトリの歴史を 1 つの project として引けるようにし、Knowledge の daily-guide にベタ書きされている旧名対応表の移設先にする
