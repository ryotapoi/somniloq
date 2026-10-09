# 専用 migrate の利用

旧履歴を残すときは、元の SQLite DB の固定 standalone snapshot と、未存在または空の移行先 DB を用意する。通常 import の `--full` は旧履歴移行の代わりにならない。

## 起動と固定 snapshot

起動形は `somniloq migrate [--config NAME_OR_PATH] --from PATH` とする。`--from` は必須、`--config` 省略時は default、位置引数なし、source 選択や `--full` は設けない。移行先は設定の `db`、再取り込み元は設定の全 Codex 入力。`--from` は呼び出し時 cwd を基準とする相対 path または絶対 path、先頭 `~/` は home 展開し、symlink 解決後の実体を使う。設定の作成・path 解決は [README の設定節](../README.ja.md#設定) を参照する。元と先が同じ canonical path または同じ file identity の hardlink なら開始前に拒否する。

`--from` には、利用者が事前に SQLite backup 等で作成して以後変更しない standalone snapshot を渡す。稼働中の DB の更新追従は対象外。既存 `-wal` / `-journal` を伴う移行元は拒否する（空でも拒否）。`-shm` の有無だけでは内容の完全性を示さないため、存在時も拒否する。snapshot を read-only で開き、SQLite の整合性確認と形状確認を行う。schema を自動修正する通常 `OpenDB` 経路を使わない。

digest は snapshot ファイル全 bytes の SHA-256、小文字 64 桁 hex。path、mtime、DB の schema revision、論理行の hash は digest の代用にしない。読み取り前後で同じ bytes digest を確認し、変化したらコピーを確定しない。元 DB への書き込み、checkpoint、schema 更新を行わない。digest が同じなら snapshot を別 path へコピーしたものでも同じ移行元。移行先の sidecar は移行先の SQLite transaction に従う。

## 受理する旧形式

旧形状名は `legacy-v013`。`PRAGMA user_version=0` であり、ユーザー定義の table は次の3個だけ、view と trigger はなし。以下に示す旧最終形状だけを受理する。v0.2 の `project_dir`、`repo_path` 欠落、source 欠落、未知追加列、未知 source、将来 revision、既に新 schema の DB は拒否する。古い整数版なら何でも受理する方式にはしない。

列の順序・宣言 type・NOT NULL・default・PK position は以下のとおり（`-` は default なし）。PK を持つ TEXT 列でも明示 NOT NULL のない列は NOT NULL=0 である。

| table | 列順（name:type:NOT NULL:default:PK position） |
| --- | --- |
| sessions | `source:TEXT:1:-:1`, `session_id:TEXT:1:-:2`, `cwd:TEXT:0:-:0`, `repo_path:TEXT:0:-:0`, `git_branch:TEXT:0:-:0`, `custom_title:TEXT:0:-:0`, `agent_name:TEXT:0:-:0`, `version:TEXT:0:-:0`, `started_at:TEXT:0:-:0`, `ended_at:TEXT:0:-:0`, `imported_at:TEXT:1:-:0` |
| messages | `uuid:TEXT:0:-:1`, `source:TEXT:1:-:0`, `session_id:TEXT:1:-:0`, `parent_uuid:TEXT:0:-:0`, `role:TEXT:1:-:0`, `content:TEXT:1:-:0`, `timestamp:TEXT:1:-:0`, `is_sidechain:BOOLEAN:0:FALSE:0` |
| import_state | `jsonl_path:TEXT:0:-:1`, `source:TEXT:1:-:0`, `file_size:INTEGER:0:-:0`, `last_offset:INTEGER:0:-:0`, `imported_at:TEXT:1:-:0` |

全 table は通常 rowid table（WITHOUT ROWID / STRICT でない）。各 source 列は `CHECK(source <> '')`。messages は `(source,session_id)` から sessions の同列への複合 FK（更新・削除 NO ACTION）。index は各 PK の SQLite 自動 index と `messages_session_idx(source,session_id)`（非 unique、全行、昇順）だけ。他の unique 制約・CHECK・FK・index はない。空白や SQL keyword 大文字小文字の差は許すが制約の意味は同じでなければならない。`sqlite_*` 内部 object はユーザー object 数に含めない。

形状が一致しても整合性確認、FK 確認、非空 source/session ID/uuid と source=`codex|claude_code|cursor_agent` のデータ確認に失敗した snapshot は拒否する。空本文や欠落日時の空文字列は拒否しない。旧正規化値を失わず限定コピーすることが目的で、任意の壊れた DB の修復は行わない。行の text は SQL プレースホルダを使う。

## 旧履歴の参照

初回の全旧会話は入力未知の legacy 会話として保持する。REF は `slq1:legacy:<snapshot_sha256>:<source>:<base64url_legacy_session_id>`。末尾は旧 session ID の UTF-8 bytes を URL-safe base64、padding なしで encode する。正常 REF の JSON array encode と混同しない。この namespace は入力不明の旧値を通常入力の同名会話と衝突させない。未知旧会話は文字列 ID、近い時刻、cwd、path 包含だけで正常会話へ吸収しない。

再実行時に以前保存した本人 group の rollout の一部が欠けていれば、その group の置換は失敗として既存の正常本文・旧行・cursor を保持する。他の独立した group は処理を続ける。

残存ログで入力と物理行を一意に証明できた Codex 本人だけを置換する。ログ欠落・所属不明・他 source の旧履歴は legacy として残す。同じ snapshot と完了 receipt がある移行先には再実行できる。移行元と元ログは変更しない。

## 成功・失敗と出力

正常な初回コピー後は独立した本人 group を最後まで処理し、一つの group の失敗で成功済み group を巻き戻さない。stdout は一つの JSON object、stderr は `migrate: skip:`, `migrate: warning:`, `migrate: error:` で区別する診断（本文を含めない）。summary field は `snapshot_sha256`, `copy_performed`, `groups_replaced`, `groups_skipped`, `groups_failed`, `legacy_messages_removed`, `legacy_messages_retained`, `legacy_conversations_retained`, `legacy_retention_warnings`, `legacy_replacement_failures`。`groups_skipped` は安全な本人本文なし group、`legacy_retention_warnings` は新本人会話を保存できたが同名旧行の入力所属を証明できず保持した会話、`legacy_replacement_failures` は残存旧行の所属証拠が競合した会話を数える。入力未知のため旧行を保持した会話も retained に含める。

本人本文がなくても、旧同名履歴・当該物理行に対応する旧行・当該入力/本人の保存済み本文や文脈・当該 rollout の保存済み cursor がないこと、解析とログ集合/内容に問題がないことを確認できればスキップする。会話保存・cursor 前進・旧行削除は行わない。既存本文の欠落など保護対象がある本人本文なし group は `no_own_messages` の失敗として保持する。

処理失敗がなく、安全なスキップと保持警告だけなら終了0。初回コピー/再実行先拒否、対象 group の解析・保存・不完全入力、ログ集合/内容の変化、所属証拠競合は終了1。警告と失敗が混在しても終了1。ログが全くない履歴の単純保持は失敗ではない。引数・設定エラー、非対応の移行元/先 schema は終了2。I/O、snapshot/receipt 不一致、既存移行先の拒否は終了1。copy 前の拒否では stdout は空。

```text
somniloq migrate --config archive --from ./archive-snapshot.db
{"snapshot_sha256":"<64 hex>","copy_performed":true,"groups_replaced":2,"groups_skipped":0,"groups_failed":0,"legacy_messages_removed":5,"legacy_messages_retained":3,"legacy_conversations_retained":4,"legacy_retention_warnings":0,"legacy_replacement_failures":0}
```

上例は説明用 digest placeholder。旧 parent の未照合残行は、本人 parent の残存 rollout がない限りログ欠落履歴の保持であり失敗にしない。残存本人 ID と同名の旧会話に所属不明行がある場合は入力間の証拠競合がなければ旧行を残して `legacy_retention_warnings` を増やし、`old_input_membership_unknown` の警告を出す。本文・role・時刻が一致しても旧 path/物理行由来 UUID の証明に代えない。再実行で snapshot 不一致なら `migrate: snapshot digest mismatch` を stderr に出し終了1。通常新 DB なら `migrate: destination has no completed copy receipt`。本文や snapshot 全内容を出力しない。
