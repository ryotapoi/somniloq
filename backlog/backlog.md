# Backlog

## この backlog の運用ルール

### バージョンと見出し

- バージョン番号は SemVer に従う（機能追加 = minor、修正のみ = patch、破壊的変更 = major）
- 見出しはバージョン単位で切る。リリースとして出す価値のあるまとまりで区切り、goal の大きさには合わせない
- バージョン見出しが大きくなったら、配下にサブ見出しを立てて塊ごとに分ける。**サブ見出し 1 つが 1 goal の実行単位**（数コミットで終わる大きさ）。小さいバージョンならサブ見出しを作らず、バージョン見出しごと 1 goal にしてよい
- **未リリースのバージョン番号は挿入・繰り下げしてよい**。v0.10.0 と v0.11.0 がある状態で v0.10.0 直後にやりたい作業ができたら、それを新 v0.11.0 とし、既存 v0.11.0 を v0.12.0 にずらす。タグを打ったバージョンは動かせない
- 番号が決まらないタスクは番号なしの見出し（例: `## docs`）に置く。次のリリースへ同梱するか独立バージョンにするかは、タグを打つときに決める
- 挙動が変わらない変更（リファクタ・テスト・ドキュメント）だけでバージョンを刻まない

### タグと CHANGELOG

- タグと GitHub Release は 1:1 で作る
- CHANGELOG は**リリース時にまとめて書く**。そのバージョンの項目が全て `[x]` になってから `## vX.Y.Z — YYYY-MM-DD` に書く
- `## Unreleased` 見出しは常設しない（空の見出しを置いておかない）。リリース前に書き溜める必要が出た例外時だけ足し、リリース時にバージョン見出しへ畳む
- CHANGELOG は英語（`CHANGELOG.md`）と日本語（`CHANGELOG.ja.md`）の両方を同じコミットで更新する
- **書くときは各 commit の diff を読む**。backlog のタスク文をそのまま写さない。内部整理のつもりの項目でも、エラー文言等のユーザー影響が出ることがある
- 同一バージョン内で「A を作って後で X に変えた」場合は、A に触れず X だけを書く。前のバージョンの A を変えた場合は変更として書く
- 完了項目は `- [x]` にして残し、そのバージョンをリリースしたら見出しごと削除する（内容は CHANGELOG と commit に残る）。**削除は明示指示があるときだけ**

## v0.9.1

- [x] TSV / Markdown / summary の正常出力で発生した write error を捨てず、各 command の error と非0 exit codeへ伝える
  - 対象は `sessions`、`projects`、`outline`、`search`、`import`、`backfill`、`show` の正常 stdout。usage、確認プロンプト、診断 stderr は対象外
  - write error より前に出力済みの内容は巻き戻さなくてよいが、command は成功扱いしない
  - 正常に書き込めた場合の出力内容・形式・exit codeと、既存のJSON出力のerror処理は変えない
  - 新しい汎用出力 abstraction は追加せず、既存のcommand / formatter内でerrorを返す
  - write errorを返すwriterを使い、直接TSVを書き出す経路とMarkdown formatter経路の回帰テストを追加する

- [x] `internal/core/db_query.go` と対応テストを、import state、sessions / projects、messages / summary、search の責務単位へ分割する
  - `internal/core` package、公開 API、SQL、振る舞いは変更しない
  - 共有する型や helper は中心となる責務の file に置き、分割のためだけの `common` / `utils` file は追加しない
  - テストも同じ責務単位に分け、既存の共通 test helper は `helpers_test.go` に維持する

- [ ] `ProcessJSONL` の `Flush`、`UpsertImportState`、`Commit` 失敗を回帰テストで保護する
  - 各失敗でエラーが返り、`NewOffset` が実行前の値から進まない
  - transaction が rollback され、失敗位置より後の処理が実行されないことを call count で確認する
  - production API や production code は変更しない

- [ ] `AGENTS.md` の `docs/specs/` を「未配置」とする記述を、実在する `docs/specs/jsonl-schema.md` と矛盾しない説明へ直す
  - JSONL ingest を変更する agent が `docs/specs/jsonl-schema.md` を仕様照合先として発見できる
  - 情報源一覧で同じ説明を不必要に重複させない

- [ ] ADR 0007 の shared ingest runner という決定が維持され、ADR 0009 / 0010 は契約の一部だけを変更したと分かる Status に直す
  - ADR 本文を現在仕様へ書き換えず、決定時点の理由を維持する
  - ADR 0009 / 0010 の Context と矛盾しない
