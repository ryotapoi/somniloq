# v0.14.0 migration 合成 fixture

[専用移行契約](../../../../docs/migration.md) 用の合成 oracle。値と本文はすべて架空で、schema と UUID 算式以外は実 DB の複製ではない。

`legacy.sql` を新しい SQLite DB へ実行し、接続を閉じる。その固定 DB の全 bytes SHA-256 を一度計算して以後同じファイルを使う。SQL ファイル自体の digest ではない。`expected.json` の digest 記述はこの生成手順を指し、全環境で SQLite ファイルが同じ bytes になると要求しない。

`physical_paths` は旧 UUID 算出時の架空 absolute path。fixture harness は JSONL をこの path に置くか、path への読み取りを fixture file へマッピングする。リポジトリ内の実 path を UUID 算出へ混ぜない。case は独立した初期状態から検証し、`mutation` は記載 file/line/DB に対してその case のみ適用する。競合 case は `override_inputs` の親root `/fixture` と子root `/fixture/shared` により、同じ物理行が二つの入力へ対応する。`candidate_input_ids` はこの照合結果。入力キーが異なるネストrootを許す設定でも、旧行所属を推測で選ばない。

| 旧 rowid | 意味 |
| --- | --- |
| 1 | 子ファイル3行目、ordinal境界未満の継承。旧 parser は埋め込み親 metadata により parent に保存 |
| 2 | 子ファイル4行目、ordinal境界以降の本人。旧保存先も誤って parent |
| 3 | 同じ旧 parent 会話の残存ログなし行。1/2の一致だけで消してはいけない |
| 4/5 | 同本人 multi の二つの rollout。新ID m1重複は全文一致のみで除き、m2は残る |
| 6 | 入力の物理証拠が競合する旧行 |
| 7 | Claude Code の保存値。Codexの置換が削除しない |
| 8 | Cursor Agent の未知日時保存値。日時補完しない |

`missing` は metadata-only の旧会話であり、本文件数0でも参照対象として保持する。`unattributed_old_same_id` は別caseの初期変種として未照合の旧 `child` 会話を追加する（本文・role・時刻は新本文と一致し、UUID は残存 path と異なる旧 `/fixture/old/01-child.jsonl` の4行目由来）。新 `child` の正常保存で同名旧行を削除し、別 ID の未照合旧行は保持する。`inherited_only_no_prior_history` は ordinal 3 が本人境界82未満で、本人本文0件・継承文脈ありの正常置換として初回/再実行とも会話と移行 cursor を保存する。`all_normal_records_ignored` は metadata-only の正常0件として同名旧本文を削除する。invalid JSON、前metadata本文、入力競合、同payload ID不一致は失敗時保持の境界例で、元ログにすべて存在したとの主張ではない。

本 fixture の確認は SQL が現行 exact schema と一致すること、全JSON/JSONLがcaseの意図どおり読めること、物理行と旧UUIDが一致すること、ordinal 7/8 と本人境界8の対応、m1の全文一致と m2の順序を対象とする。production の原子性と再実行は core/CLI の合成 fixture テストで確認する。実 DB 全体、性能、実使用は対象外。
