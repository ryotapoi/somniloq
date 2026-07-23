# ADR 0007: adapter ProcessFile の共通骨格を ingest runner に抽出する

## Status

Accepted

## Context

claudecode / codex 両 adapter の `ProcessFile` は、open / seek / offset 追跡 / `ReadBytes` ループ / `hasBody` 判定 / `import_state` upsert / commit がほぼ同型に重複していた。さらに codex は `scanPrefix` に 3 つ目の同型ループを持っていた。3rd source を追加すると同じ骨格が 3 重になり、骨格側のバグ修正・契約変更（例: 走査エラーの非致命化）が全 adapter への同期変更になる。

## Considered Options

- **A: 共通ループの抽出（runner + stateful handler）**: I/O・offset・トランザクション・import_state の骨格を `ingest.ProcessJSONL` に集約し、行の解釈・per-file 状態（claudecode の title/agent-name buffer、codex の session_meta / lineNumber）は `FileHandler` interface（`Begin` / `HandleLine` / `Flush`）として adapter 側に残す。
- **B: adapter 責務を行 → record 変換に縮小**: adapter は行を normalized record / event 列へ変換するだけにし、書き込みも共通層が担う。
- **C: 現状維持（重複容認）**: 骨格の重複を別々に変わりうる知識として残す。

## Decision

We will extract the shared skeleton into `ingest.ProcessJSONL` with a `FileHandler` interface, keeping record interpretation and per-file state in each adapter (Option A). Line iteration itself is shared as `ingest.ForEachLine`, which codex's prefix recovery (`Begin`) also uses, so the former 3 つの同型ループは 1 箇所になる。

B は却下: claudecode の「title/agent-name を buffer して EOF で flush」、codex の「session_meta 到着まで message を捨てる」という書き込みタイミングの source 固有意味を event 列として共通層に表現する必要があり、共通層が各 source の知識を持つことになって意味がぼやける。
C は却下: 骨格は「incremental JSONL import」という同一目的を持ち、将来変更（offset 計算修正、走査エラー契約変更）が常に全 adapter へ同時に効くため、共通化の条件（同じ目的・同じ将来変更）を満たす。

挙動保存上の重要点: runner は空行を skip せず生の行を handler に渡す。codex の lineNumber は空行も数えて message UUID の生成に使われるため、骨格側で行を間引くと再 import 時に UUID が変わり既存データと不整合になる。

## Consequences

- 3rd source の追加は「ScanFiles + FileHandler 実装 + parser」だけになり、トランザクション・offset 管理を再実装しなくてよい。
- 骨格の契約変更（backlog の走査エラー非致命化など）が `ProcessJSONL` 1 箇所の変更になる。
- `Adapter` interface（`ScanFiles` / `ProcessFile`）という core から見える境界の形は維持される。source identity は `importSourceSpecs` と各 adapter の `ProcessJSONL` 呼び出しで決まる。戻り値の契約はその後 ADR 0009（`ProcessResult`）と ADR 0010（`ScanFiles` の非致命エラー）で変更した。
- handler は per-file 状態を持つため、`ProcessFile` 呼び出しごとに新しい handler を生成する規約が増える（adapter 自体は stateless を維持）。
