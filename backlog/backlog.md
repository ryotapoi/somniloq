# Backlog

## v0.2

- [x] `--summary` を件数指定に対応させる
  - `--summary N` (デフォルト 0 = 無効): user メッセージを時系列順に先頭 N 件表示
  - `--summary` は session-id モード / time-range モード両方で有効
- [x] `--summary` 出力から `/clear` を省く
  - user メッセージの本文が `<command-name>/clear</command-name>` または `<local-command-caveat>` で始まるものをスキップ
  - `--include-clear`: 上記 2 つのスキップのみ無効化（sidechain は常に除外、tool_result は import 時点で除外済み）。`--include-clear` 単独はエラー
