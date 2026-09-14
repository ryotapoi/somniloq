# ADR 0018: source を識別した横断参照と未知 metadata の条件契約

## Status

Accepted（2026-09-14 決定）

## Context

Cursor Agent のログは timestamp と repository を持たないことがある。同じ DB には source ごとに同一の session ID も保存できるため、一覧や検索の結果をどの source の会話として読むかを識別できる必要がある。一方、欠落 metadata を推測して条件一致に使うと、保存されていない事実を検索結果へ混ぜてしまう。

## Decision

`sessions` と `search` の TSV は末尾に内部 source 値（`claude_code`、`codex`、`cursor_agent`）を出し、show の Markdown metadata に Source を出す。JSON の既存 source フィールド、projects の source 横断 repository 集約、outline の出力 shape は維持する。

時刻条件が指定されたときは NULL / 空 timestamp を一致させず、project 条件が指定されたときは NULL / 空 repository を一致させない。条件なしの参照と projects の空 repository 集約では、未知値を保存されたまま扱う。session ID が複数 source にある場合は既存の曖昧エラーを維持し、暗黙に source を選ばない。

## Consequences

- TSV 消費側は追加された末尾 source 列を扱う必要がある。
- unknown metadata を補完する source 別分岐、source filter、projects の source 別集約は導入しない。
- 時刻または project 条件が必要な利用では、unknown metadata の Cursor 会話は意図的に結果外となる。
