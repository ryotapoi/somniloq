# ADR 0011: outline サブコマンドとターン採番契約

## Status

Accepted

## Context

長いセッションは全文 `show` の前に構造を掴む手段がなく、Knowledge 側の /jot は「全文をファイルへ出して subagent に時系列地図を作らせる」手順で代替していた。backlog v0.6 はアウトライン表示の追加を求め、`somniloq outline <session-id>` か `show --toc` かの形は実装時判断に委ねた。同じ v0.6 に「`show --turn 40..60` の部分読みはアウトライン表示と同一の採番を共有すること」という後続項目があるため、採番規則は単発の表示仕様ではなくコマンド間で共有される契約になる。

また、旧形式の Codex rollout は per-record timestamp を持たず全レコードが同一 timestamp になるため、`ORDER BY timestamp ASC` 単独ではタイ行の順序が SQLite 上不定で、採番が実行ごとに揺れうることが実装時に判明した。

## Considered Options

- **独立サブコマンド `outline`**: 出力形（TSV 一覧）が show（Markdown 全文）と異なり、show のフラグ群（`--summary` 等）と直交しない。後続 backlog 項目から名前で参照しやすい
- **`show --toc` フラグ**: コマンド数は増えないが、show のフラグ空間に出力形の異なるモードが混ざる
- **採番を user メッセージのみの列で行う**: outline 単体では単純だが、`show --turn` がメッセージ全文を扱うときに別ロジックの採番が必要になり、ズレの温床になる

## Decision

We will expose turn-based session structure as a dedicated `outline` subcommand, with turn numbers defined by a single shared rule: walk the full GetMessages output (chronological, sidechain excluded), increment the 1-based turn at each user message, and fold messages before the first user message into turn 1. Synthetic user messages (`/clear` echo, `<local-command-caveat>`) count as turns and are displayed, keeping the numbering 1:1 with the message sequence. GetMessages / GetSummaryMessages order by `timestamp ASC, rowid ASC` so the numbering is deterministic for tied timestamps.

## Consequences

- `show --turn` や JSON 出力などの後続機能は `assignTurns`（cmd/somniloq/turn.go）を同じ入力（GetMessages の全列）に適用するだけで採番が一致する
- 採番をフィルタ済み列（GetSummaryMessages 等）に適用すると契約が壊れるため、turn 系の新機能は必ず全列を入力にする必要がある
- 合成 user メッセージがアウトラインの行として並ぶ。ノイズになる場合は「採番には含めて表示だけ抜く」拡張（表示フィルタ）で対応でき、採番は変わらない
- rowid タイブレーカーにより `messages` は INSERT OR IGNORE（置換なし）を維持する前提が生まれる（llm-wiki/ に記録）
