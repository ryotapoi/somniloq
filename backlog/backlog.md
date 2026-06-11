# Backlog

## v0.6.0

2026-06-11 Knowledge 側の運用（/jot /retro /rem、LLM Wiki 連携の見据え）からの機能追加。順序は上から。

- [x] セッションのアウトライン表示を追加する: ユーザーメッセージだけを「ターン番号・時刻・先頭 1 行」で時系列に並べ、長いセッションの構造を全文 show する前に掴めるようにする（`somniloq outline <session-id>` を採用）。sidechain 除外は show と同じ扱い。Knowledge の /jot が「長セッションは全文をファイルに出して haiku subagent に時系列地図を作らせる」手順で代替している箇所の置き換え先
- [x] show の部分読みを追加する: `show <session-id> --turn 40..60` / `--tail <N>` のようにターン範囲を指定して読めるようにする。アウトラインで見つけた範囲だけ読む用途。ターン番号はアウトライン表示と同一の採番を共有すること
- [x] sessions 一覧にサイズ列を追加する: セッションの本文合計サイズ（バイト数を採用）を列に出し、show する前に「大きいセッションか」を判定できるようにする。MessageCount だけでは 1 メッセージが巨大なセッションを見分けられない
- [x] `--format json` を追加する: sessions / show / projects（アウトラインも）で JSON 出力を選べるようにする。Knowledge の list-sessions.sh が show の Markdown を awk でパースしている脆さの解消先
- [x] search コマンドを追加する: `somniloq search <query> [--since] [--until] [--project]` で、セッション ID・時刻・マッチ前後のスニペットを返す。実装は LIKE 全走査でよい（本文 42 MB の現 DB で実測 0.11 秒。FTS5 は日本語だと trigram 必須で、索引が本文の 2〜3 倍に膨らむ・3 文字未満のクエリが索引で引けない制約があるため、LIKE で困るスケールになるまで見送り）
- [x] project alias 設定を追加する: `~/.somniloq/config.json`（`--config` フラグで上書き可、JSON なので依存追加なし）に `projectAliases`（例: `"brimday": ["whisday"]`）を持ち、`--project` 指定時にグループ内のどの名前でもマッチするよう展開する。リネーム済みリポジトリの歴史を 1 つの project として引けるようにし、Knowledge の daily-guide にベタ書きされている旧名対応表の移設先にする
