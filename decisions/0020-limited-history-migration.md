---
status: superseded
superseded_by: 0024-owner-history-migration.md
---

# ADR 0020: 履歴保持のための限定移行


## Context

ADR 0019 は専用 backfill と旧 DB 救済経路の保守負担を避け、元ログから新 DB を作る運用を選んだ。v0.14.0 では入力 identity と本人会話のモデルが変わる一方、元ログが失われた保存済み履歴も利用可能な形で残す要求が確定した。元ログの再 import だけでは、その履歴を失わずに新モデルへ移せない。

旧 Codex UUID は物理 path と行番号を示すが、旧 session ID は埋め込み親 metadata の影響を受ける。旧 ID ごとの一括置換では子の出自と本人帰属を混同し、所属不明行を消すおそれがある。入力や block・実日時を推測で補う方法も、旧保存値を原文の証拠として誤って扱う。

## Decision

既知の旧最終形状だけを固定 snapshot から別 DB へ限定コピーする専用 migrate を採用した。未知履歴を legacy namespace に残し、残存 Codex ログが一意に証明する物理行だけを、本人 group の全文保存と原子的に置換する理由は、ログ欠落の履歴保持と既知本人の原文利用を両立させるためである。snapshot digest と完了 receipt は、途中失敗後に別の旧 DB を同じ移行先へ混ぜる危険を防ぐために採用した。利用時の前提と旧 snapshot 形状は [移行手順](../docs/migration.md)、履歴保持の制約は [要件](../docs/requirements.md) に置く。

ADR 0019 の専用旧 DB 移行を追加しない判断を覆す。専用 backfill の廃止は存続する。今回必要なのは固定された一つの旧形式の履歴保持であり、保存済み値の一般補正、任意の旧版からの upgrade、稼働 DB の追従、Claude Code / Cursor Agent の再解析を維持する理由はない。通常 import の部分成功や full 再構築にこの厳密な旧行置換を混ぜず、明示された専用入口に限定した。
