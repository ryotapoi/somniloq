# v0.14.0 検索・取得の期待例

[確定契約](../../../../docs/specs/v0.14.0-contract.md) の代表オラクル。まとまり一覧・詳細全一致と show の例は実装済み。活動日4 mode と取り込み下限も実装済み。

`config.toml` は隣の v0.14.0 fixture を指す構文例。expected.json の完全REFは host依存を避けるため canonical root `/fixtures/codex/input-a` と `/fixtures/codex/input-b` に対して計算している。実ホストでの設定実行は実際の実体rootに対するREFへ置き換える。

root/child検索例は input-a の root.jsonl、child.jsonl、child-extra.jsonl、grandchild.jsonl、no-boundary.jsonl、ordinal-no-boundary.jsonl を本人集合とする。意図的な malformed `missing-ordinal.jsonl` はこの成功プロファイルから外し、移行失敗例として別検証する。単独child.jsonlの番号と、複数rollout統合後の番号は expected.json の説明どおり区別する。旧形式2会話は日時未知。

regexp期待区間はGo標準regexpのUTF8 byte区間を直接確認する。duplicate intervalsはpatternIndexesをまとめ、cross-pattern overlapは別箇所として保つ。zero widthは許可、空patternはCLIで拒否する。`o*` の隣接した末尾空一致が省略されるのはGo APIの挙動。

日時・project/imported-since の例は設計上の境界条件で、実ログの観測事実ではない。date bounds例は明示したstart/lastだけが既知の2発言会話を想定する。show selectorのR/Cはrefs欄を参照し、残りの略記は各identityの完全REFに置き換える。順序・重複排除・0件・末尾超過・one-line後処理は core/CLI テストで検証する。

lineText の LF 境界は search_matches_test.go の具体例で保護する。`one\ntwo` の LF 一文字一致 `[3,4)` は `one`、LF 直後のゼロ幅 `[4,4)` は `two`、`one\n` の末尾ゼロ幅は空の末尾行。複数行一致は途中の LF を保持し、一致の末尾が LF ならその後の行を含めない。

`expected.json` の listExample は build CLI で確認した親子 AND の一覧 envelope 例。members/matchedMembers は全6本人で、本文抜粋を含まない。importedAt は例の取り込み時点であり、実行ごとに変わる。実行時は config.toml の db を一時ディレクトリに置き、root を実体 path に設定して import 後に `search --config PATH --all -e "Inherited question" -e "Child answer" --format json` を呼ぶ。既定20件、明示0件、末尾超過の TSV/JSON も同じ config で試用できる。

日時の直接オラクルは `internal/core/search_groups_test.go` の `TestSearchGroupsActivityModesAndImportCandidates`。親子孫の min/max、gap の active/overlap 差、offset/nanosecond、未知日時、子だけ project/取り込み候補と AND を同じ SQLite fixture で検証する。CLI の日付 upper・設定上書き・厳密入力・filter 後 total/page は `cmd/somniloq/search_test.go` で保護する。詳細は active のみ、非 active mode は一覧専用として拒否し、取り込み下限は共通候補条件とする。
