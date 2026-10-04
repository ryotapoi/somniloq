# v0.14.0 検索・取得の期待例

[未実装契約](../../../../docs/specs/v0.14.0-contract.md) の代表オラクル。production の現行 CLI がこの出力を返すという意味ではない。

`config.toml` は隣の v0.14.0 fixture を指す構文例。expected.json の完全REFは host依存を避けるため canonical root `/fixtures/codex/input-a` と `/fixtures/codex/input-b` に対して計算している。実ホストでの設定実行は実際の実体rootに対するREFへ置き換える。

root/child検索例は input-a の root.jsonl、child.jsonl、child-extra.jsonl、grandchild.jsonl、no-boundary.jsonl、ordinal-no-boundary.jsonl を本人集合とする。意図的な malformed `missing-ordinal.jsonl` はこの成功プロファイルから外し、移行失敗例として別検証する。単独child.jsonlの番号と、複数rollout統合後の番号は expected.json の説明どおり区別する。旧形式2会話は日時未知。

regexp期待区間はGo標準regexpのUTF8 byte区間を直接確認する。duplicate intervalsはpatternIndexesをまとめ、cross-pattern overlapは別箇所として保つ。zero widthは許可、空patternはCLIで拒否する。`o*` の隣接した末尾空一致が省略されるのはGo APIの挙動。

日時・project/imported-since の例は設計上の境界条件で、実ログの観測事実ではない。date bounds例は明示したstart/lastだけが既知の2発言会話を想定する。show selectorのR/Cはrefs欄を参照し、残りの略記は各identityの完全REFに置き換える。順序・重複排除・0件・末尾超過・one-line後処理を第4〜6段階の実行検証へ移す。
