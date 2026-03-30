# Backlog

- [x] import コマンド（JSONL パース + SQLite スキーマ + 差分取り込み）
- [x] sessions コマンド（一覧表示）
- [x] show コマンド（Markdown 出力）
- [x] `import --full` 実行時の確認ステップ（`y/N` プロンプト、`--yes` でスキップ）
- [x] `--since` で絶対日付指定に対応（`2026-03-28`、`2026-03-28T15:00`）
- [x] `--until` オプション追加（範囲指定）
- [x] `projects` コマンド（プロジェクト一覧 + セッション数、`--since` 対応）
- [x] cclog → somniloq リネーム（モジュールパス、パッケージ名、DB パス、ドキュメント、スキル名）
- [x] ローカルタイムゾーン対応（--since/--until の入力と sessions/show の出力をローカルタイムに）
- [x] サブコマンド --help（`somniloq sessions --help` 等でフラグ一覧表示）
- [x] show --summary（各セッションの冒頭だけ出すモード）
- [x] sessions --short（プロジェクト名を最後のパス要素のみに）
- [x] show で content が空のメッセージ（tool_use のみ等）をスキップする
- [ ] サンプルスキル（examples/skills/somniloq/）
- [ ] README.md
