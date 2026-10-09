---
status: current
---

# ADR 0025: 本人 ID に限定した旧履歴置換

## Context

旧 Codex parser は継承文脈や対象外イベントを本人本文として保存し、別 ID に保存することもあった。現在の本人原文を優先する同名置換は必要だが、別 ID の物理行照合はログに本人 ID がない旧履歴まで削除する。root 全体の再列挙と旧行の逐次照合は、独立した本人 group の処理を他会話数・旧履歴件数に掛け算で依存させていた。

## Decision

既知の旧最終形状だけを固定 standalone snapshot から別 DB へ限定コピーする専用 migrate を維持する。正常解析した Codex 本人 group は本人本文0件も正しい結果として確定し、今回の snapshot・source・本人 ID が一致する legacy 本文・会話を削除する。旧 UUID の一致を条件にしない。継承文脈を本人本文へ昇格せず、本人原文を優先するためである。

別 ID の旧行は物理行が一致しても残す。現ログに本人 ID がない旧履歴と他 source は入力未知の legacy namespace に保持し、入力・親子関係・実日時を推測で補わない。

root は開始時に列挙し、本人 index の後は一 group ずつ全文を読む。各 group の読み取り前・commit 前にはその rollout の内容だけを照合し、無関係なログの追加・削除で独立 group を失敗させない。初期走査・解析失敗、入力競合、対象 rollout の変化、保存済み rollout の欠落では該当入力または group の既存状態を保持する。index で全件本文を常駐させないのは大きな移行のメモリ境界を保つためである。

本人全文・文脈・関係・移行 cursor の保存と旧履歴削除は一 transaction で確定する。保存・commit 失敗でも部分確定せず、独立した先行成功 group は保持する。元 DB と元ログは変更しない。snapshot bytes の digest と完了 receipt を照合し、同じ snapshot の再実行でコピーを繰り返さない。別の旧 DB を同じ移行先へ混ぜる危険を防ぐためである。

ADR 0019 の専用旧 DB 移行を追加しない判断を覆す方針と、専用 backfill の廃止は存続する。任意の旧版からの upgrade、稼働 DB の追従、一般的な保存値補正、Claude Code / Cursor Agent の再解析は提供しない。通常 import の部分成功や full 再構築に移行条件を混ぜない。利用時の前提と旧 snapshot 形状は [移行手順](../docs/migration.md)、履歴保持制約は [要件](../docs/requirements.md) に置く。
