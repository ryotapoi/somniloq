# ADR 0006: import の source 選択

## Status

Accepted（2026-05-05 決定）

## Context

v0.4 リリース前に CLI の取り込み入口を整理する。ADR 0005 では `import` と `import-codex` を分ける方針にしたが、Claude Code / Codex を同じ SQLite DB に保存・検索するという mission では、通常の取り込み操作も source 横断が自然である。

リリース前のため、互換性維持のためだけに `import-codex` を残す必要はない。

本 ADR が覆すのは ADR 0005 の CLI サブコマンド方針だけで、ADR 0005 の残る決定——保存対象レコード・content 抽出・`session_id` の出所・`messages.uuid` の導出・差分取り込み時の `session_meta` 読み直し・adapter の配置——は当時の決定のまま有効で、現在の実装もそれに従っている。

## Considered Options

- **`import` を統合入口にし、`--source all|claude-code|codex` で絞る**: 通常は両 source を取り込み、必要なときだけ対象を絞れる。CLI の入口が増えず、同一 DB に集約する目的と揃う
- **`import` / `import-codex` を併存する**: source ごとの入口は明確だが、通常運用で2コマンド実行が必要になり、リリース前から互換 shim を抱える

## Decision

- `somniloq import` はデフォルトで Claude Code と Codex の両方を取り込む
- 対象を絞る場合は `--source all|claude-code|codex` を使う
- `import-codex` は CLI サブコマンドとして残さない
- Claude Code のデフォルトパスは `~/.claude/projects/`、Codex のデフォルトパスは `~/.codex/sessions/` のまま維持する

## Consequences

- 通常の差分取り込みは `somniloq import` だけで済む
- source 固有の JSONL パースやファイル走査は引き続き adapter 配下に閉じる
- `--full` は既存の文言どおり DB 全体を削除してから、選択した source を再取り込みする
- `import-codex` を使う既存手順は v0.4 リリース前に README、scope、example skill から削除する

## References

- `docs/rules/scope.md`
- `backlog/backlog.md`
- `docs/decisions/0005-codex-ingest-adapter-policy.md`
