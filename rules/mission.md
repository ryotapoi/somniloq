# Mission

## 目的

Claude Code のセッションログ（JSONL）を読み取り、SQLite に保存・検索するCLIツール。

## なぜ作るか

- `~/.claude/projects/` 配下の JSONL は生データで、検索性がない
- セッション横断で履歴を参照・検索したい
- CLI として呼び出し、Daily Note 等に活用する

## 非目標

- リアルタイム監視・ストリーミング処理
- Web UI や GUI
- Claude Code 以外のツールのログ対応
