# Backlog

## v0.2.1

- [x] `--summary` を件数指定に対応させる
  - `--summary N` (デフォルト 0 = 無効): user メッセージを時系列順に先頭 N 件表示
  - `--summary` は session-id モード / time-range モード両方で有効
- [x] `--summary` 出力から `/clear` を省く
  - user メッセージの本文が `<command-name>/clear</command-name>` または `<local-command-caveat>` で始まるものをスキップ
  - `--include-clear`: 上記 2 つのスキップのみ無効化（sidechain は常に除外、tool_result は import 時点で除外済み）。`--include-clear` 単独はエラー
- [x] `show` サブコマンドの usage 表記を実挙動に合わせる
  - 現状 `somniloq show <session-id> [--summary <N>] [--include-clear] [--short]` と書かれているが、Go `flag` は位置引数の後のフラグを受け付けないため `show <id> --summary 3` は `too many arguments` で落ちる
  - usage をフラグ先行の形に修正: `somniloq show [--summary <N>] [--include-clear] [--short] <session-id>` / `somniloq show [--since <time>] [--until <time>] [--project <name>] [--summary <N>] [--include-clear] [--short]`
  - 修正箇所は `cmd/somniloq/main.go` の 2 箇所（setUsage 呼び出しと `showUsage` 定数）
  - SKILL.md / README は既にフラグ先行で書かれているので修正不要
