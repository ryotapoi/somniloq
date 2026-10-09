---
status: current
---

# ADR 0023: source の証拠に基づく取り込み境界

## 決定

`cmd/somniloq` が CLI、`internal/core` が DB と検索、`internal/ingest` の source 別 adapter が JSONL の走査・解釈を担う。依存方向は cmd → core → ingest とし、ingest は core の SQL に依存しない。共通の物理行処理、未解析行の診断、末尾の再開は共有 runner に置く。保存 transaction は source 中立な interface を基本とし、Claude 固有の書き込みだけを隣接した拡張 interface に置く（[ADR 0008](0008-import-transaction-source-neutral.md)）。

本人と関係は source の明示的な証拠だけで決める。Codex は最初の有効 metadata で本人を固定し、明示された継承境界だけで親文脈を除く。Claude Code の直接親には同一物理親内の Agent/Task call、対応する result、子 agent ID の連鎖を要求する。Cursor Agent の未知日時・repository・親を slug、mtime、本文 tag、取り込み時刻で補わない。canonical な本人原文列、関係、差分 cursor の再構築は一つの transaction で確定し、失敗時に旧本文を保持する。

通常 `import` は source ごとの保存境界と部分成功を維持する。旧履歴を限定置換する専用 `migrate` の厳密条件は [ADR 0024](0024-owner-history-migration.md) に従い、通常 import と混ぜない。

## 理由

source のログ形状と本人・親を示す根拠は異なる。共通の推測規則で埋めると、継承された本文を本人として重複保存したり、未知の親を確定したものとして表示したりする。物理行の取り扱いは共有できるが、source の意味づけは adapter に閉じる。原文順と cursor を別々に確定すると、前方への追記や途中編集の後に番号と差分状態が食い違う。
