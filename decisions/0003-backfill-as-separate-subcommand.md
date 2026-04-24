# ADR 0003: repo_path バックフィルを専用サブコマンド化

## Status

Accepted

## Context

v0.3 で `sessions.repo_path` カラムを導入し、新規 import 時は `ResolveRepoPath(cwd)` で自動的に埋めるようにした。しかし v0.3 以前に取り込まれた既存セッションは `repo_path` が NULL のまま残り、`import_state` による差分 import では再読み込みされないため自然には埋まらない。

当初案は「`somniloq import` の先頭で自動的にバックフィルを走らせる」設計だったが、Codex レビューで次の問題が指摘された:

- `ResolveRepoPath` の失敗は「not a git repository」（恒久的）と「safe.directory / git 不在 / 権限」（一時的）を区別できない
- 自動実行で失敗済みセッションに空文字マーカー等を書くと、環境を直せば解決できたはずのセッションも永久に再試行できなくなる
- マーカー無しで毎回 import 時に再試行すると、解決不能な cwd（実 DB で 30〜50 ユニーク）のたびに git 起動コストが数秒〜十数秒積み上がる

## Considered Options

- **import 内で自動バックフィル + マーカー方式**: 失敗したセッションに空文字等のマーカーを書き、以降スキップ。恒久的失敗に適応するが、一時的失敗も永続化され、ユーザーが環境を直しても自動回復しない。`upsertSession` 側の NULL/非空の 2 値パターンとも非対称を作る
- **import 内で自動バックフィル + マーカー無し**: 冪等だが、毎回 import 実行で解決不能 cwd に対して git を再起動し、コストがかさむ
- **専用サブコマンド `somniloq backfill`**: `import` から切り離し、ユーザーが明示的に 1 回（または環境を直した後に再度）実行する。失敗セッションは NULL のまま残り、次回実行で自然に再試行される

## Decision

We will add a dedicated `somniloq backfill` subcommand that resolves `repo_path` for sessions where it is NULL and `cwd` is non-empty. No marker is written for unresolved sessions; they stay NULL and are retried on every invocation. `import` itself does not perform backfill.

- `internal/core/backfill.go` に `BackfillRepoPaths(db *DB) (resolved, unresolved int, err error)` を公開
- 対象抽出 → `ResolveRepoPath` での解決（cwd 単位でメモ化）→ 単一トランザクションでの UPDATE、の 3 段階で実装。`SetMaxOpenConns(1)` 下で tx を握ったまま外部プロセスを起動しないようフェーズを分離する

## Consequences

- v0.3 アップグレード後に 1 手順増える（ユーザーが `somniloq backfill` を 1 回実行）
- 失敗セッションはユーザーが環境（git 設定・インストール等）を直して再実行すれば自然に解決される
- `repo_path` カラムは従来の 2 値（NULL / 非空）のまま。`upsertSession` の `COALESCE(NULLIF(excluded.repo_path, ''), sessions.repo_path)` 条件と非対称を作らない
- 毎回の `import` に追加コストが乗らない
- `ResolveRepoPath` の失敗原因（stderr パース）分類は本タスクでは実装不要。将来必要になれば別タスクで拡張する
