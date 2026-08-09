# ADR 0016: ingest runner の判断系列を 1 件の現行 ADR に統合する

## Status

Accepted（2026-08-09 決定）

## Context

ADR 0007 は adapter の `ProcessFile` に重複していた共通骨格を shared ingest runner へ抽出した。続く ADR 0009 は `FileHandler` / `ProcessFile` の結果処理を、ADR 0010 は `ScanFiles` のエラー処理を変更した。それぞれの Context は、共有 runner を維持したまま異なる境界を変えた経緯を記録している。

しかし、3 件の Status だけでは、この系列を現在の読み手が 1 件の記録から辿れない。旧 ADR の本文は決定時点の理由であり、現在の契約や実装の正本へ書き換える対象でもない。

## Considered Options

- **A: 3 件を Accepted のまま残す**: 各判断を独立して読めるが、どの記録から系列全体を辿るかが分からない。
- **B: 旧 ADR の Status に限定付きの説明を加える**: 0007 の共有 runner と、0009 / 0010 の変更境界を Status で表せるが、Status の標準形式を増やし、経緯が複数のヘッダに分散する。
- **C: 1 件の現行 ADR に系列を統合し、旧 ADR をそこへリダイレクトする**: 旧本文を決定時点の記録として維持したまま、現在の案内と判断のつながりを 1 件に集める。

## Decision

We will adopt Option C. ADR 0016 を ingest runner の判断系列の現行案内とし、ADR 0007、ADR 0009、ADR 0010 の Status を `Superseded by ADR 0016` に更新する。

ADR 0007 の shared runner 抽出はこの系列の出発点として残る。ADR 0009 は `FileHandler` / `ProcessFile` の結果処理を変更し、ADR 0010 は `ScanFiles` のエラー処理を変更した。この ADR はそれらを現在の振る舞い・契約の正本として再記述しない。現在の実装は `internal/ingest/process.go`、`internal/ingest/ingest.go`、`internal/core/import.go` と対応テストを参照し、JSONL の入力形式は `docs/specs/jsonl-schema.md` を参照する。ADR の役割と Status の形式は `docs/rules/information-management.md` に従う。

## Consequences

- ingest runner の判断経緯は ADR 0016 から 0007、0009、0010 へ辿れる。
- 0007、0009、0010 の Context、Considered Options、Decision、Consequences は決定時点の記録として変更しない。
- 現在の振る舞いまたは契約を変更する場合は、rules、specs、コード、テストの該当する正本を更新し、理由が必要なら新しい ADR を追加する。
