# Mission

## 目的

Claude Code と Codex のセッションログ（JSONL）を読み取り、SQLite に保存・検索する CLI ツール。

## なぜ作るか

- `~/.claude/projects/` や `~/.codex/sessions/` 配下の JSONL は生データで、検索性がない
- ツール横断・セッション横断で履歴を参照・検索したい
- CLI として呼び出し、Daily Note 等に活用する

## 非目標

- リアルタイム監視・ストリーミング処理
- Web UI や GUI
