# ADR 0006: import の source 選択

## Status

Accepted

## Context

v0.4 リリース前に CLI の取り込み入口を整理する。ADR 0005 では `import` と `import-codex` を分ける方針にしたが、Claude Code / Codex を同じ SQLite DB に保存・検索するという mission では、通常の取り込み操作も source 横断が自然である。

リリース前のため、互換性維持のためだけに `import-codex` を残す必要はない。

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

- `rules/scope.md`
- `backlog/backlog.md`
- `decisions/0005-codex-ingest-adapter-policy.md`
