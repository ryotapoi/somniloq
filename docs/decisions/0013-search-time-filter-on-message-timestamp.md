# ADR 0013: search の時刻フィルタはメッセージ timestamp 基準

## Status

Accepted

## Context

`search`（v0.6）は `--since`/`--until` を持つ。既存の `sessions` / `show` / `projects` の時刻フィルタはセッションの `started_at` 基準で実装されている（`timeFilterConditions`）。search も同じ基準にするか、マッチ対象であるメッセージの `timestamp` 基準にするかで結果が変わる: 長いセッションでは開始から数日後のメッセージもあり、セッション開始基準だと「先週書いた内容」を `--since 7d` で探したとき、8 日前に始まって 6 日前に書かれたメッセージが漏れる。

## Considered Options

- **A: セッション `started_at` 基準**: 既存コマンドと同じ解釈・同じ実装（`timeFilterConditions` 再利用）
- **B: メッセージ `timestamp` 基準**: 検索対象の行そのものの時刻で絞る

## Decision

We will filter search results by the message `timestamp`. 検索の対象はメッセージであり、ユーザーが `--since 7d` に期待するのは「その期間に書かれた内容」だから。セッション一覧系コマンドの対象はセッションなので `started_at` 基準のままでよく、コマンドごとに「フィルタはそのコマンドの主対象の時刻に掛かる」という一貫した規則になる。

## Consequences

- `--since`/`--until` の意味がコマンド間で字面上は揺れる（sessions はセッション開始、search はメッセージ時刻）。規則は「主対象の時刻」で一貫しており、scope.md の search 節に明記して吸収する
- `timeFilterConditions` は信頼済みの内部時刻カラムを受ける。sessions は `s.started_at`、search は `m.timestamp` を共通の filter builder に渡し、時刻列の違いを明示したまま Since/Until/Projects の組み立てを共有する
- 旧形式 Codex rollout は全メッセージが `session_meta` の timestamp を継承するため、実質セッション開始基準と同じ挙動になる（劣化ではなく同等）
