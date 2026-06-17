# ADR 0008: ImportTransaction を source 中立に縮小し固有書き込みは拡張 interface にする

## Status

Accepted

## Context

`ingest.ImportTransaction` に Claude Code 固有の `UpdateSessionTitle` / `UpdateSessionAgentName` が載っていた。codex はこれらを使わず、source が増えるたびに各 source 固有メソッドが共通 interface に積み上がる union interface 化が進む構造だった。

## Considered Options

- **A: 共通 interface を最小化 + source 固有の拡張 interface**: `ImportTransaction` は全 source が使う 5 メソッド（UpsertSession / InsertMessage / UpsertImportState / Commit / Rollback）に縮小。title/agent-name は `claudecode.SessionMetaWriter` として claudecode パッケージに定義し、Flush 時に type assertion で取得する。
- **B: union interface のまま残す**: 現在 2 source・固有メソッド 2 つなので実害が出るまで放置する。
- **C: 固有メソッドをトランザクション外の DB API として渡す**: adapter constructor が title 更新関数を別依存として受け取る。

## Decision

We will shrink `ImportTransaction` to the source-neutral five methods and define `claudecode.SessionMetaWriter` as an extension interface next to the only adapter that needs it (Option A). Type assertion はランタイムチェックだが、`internal/core` に `var _ claudecode.SessionMetaWriter = importTx{}` を置いてコンパイル時に担保する（core は既に claudecode へ依存しており、依存方向は変わらない）。

B は却下: title/agent-name が claude-code 固有である意味境界は現在すでに存在しており、共通 interface に置くと codex（および将来の source）に無関係なメソッドが見え続ける。
C は却下: title 書き込みは同一トランザクション内で行う必要があり、トランザクション外の依存として渡すと commit 境界が分裂する。

## Consequences

- 新 source の adapter は source 中立な 5 メソッドだけを前提にでき、他 source の知識を見ない。
- source 固有の書き込みが必要になったら、その source のパッケージに拡張 interface を足し、core に `var _` チェックを 1 行足すだけでよい。
- type assertion の失敗は理論上ランタイムエラーだが、`var _` チェックにより core の実装変更ではコンパイルエラーとして検出される。
