---
status: current
---

# ADR 0024: 正常解析した本人の旧履歴置換

## Context

旧 Codex parser は継承文脈や対象外イベントを本人本文として保存し、埋め込み親 metadata の影響で別 ID に保存することもあった。ADR 0020 の物理行一致だけによる削除では、残存ログから正常に本人本文0件と判明しても同名の旧本文が残り、現在の原文と旧保存値が食い違う。

## Decision

既知の旧最終形状だけを固定 standalone snapshot から別 DB へ限定コピーする専用 migrate を維持する。正常解析できた Codex 本人 group は本人本文0件も正しい結果として確定し、同じ ID の Codex legacy 本文・会話を全削除する。旧 UUID の一致を同名置換の条件にしない。旧保存値より現在の本人原文を優先し、継承文脈を本人本文として残さないためである。

別 ID の旧行は残存ログが入力と物理行を一意に証明する行だけを削除する。子の物理行が旧 parent に保存されていても、未照合の parent 残行を巻き込まない。ログに本人 ID がない未照合旧履歴と他 source は入力未知の legacy namespace に保持する。入力・親子関係・実日時を推測で補わない。

本人全文・文脈・関係・移行 cursor の保存と旧行削除は一 transaction で確定する。解析・保存失敗、不完全入力、入力競合、保存済み rollout の欠落、ログ集合/内容の変化では既存状態を保持し、独立した正常 group は処理を続ける。元 DB と元ログは変更しない。snapshot bytes の digest と完了 receipt を照合し、同じ snapshot の再実行で旧コピーを繰り返さない。別の旧 DB を同じ移行先へ混ぜる危険を防ぐためである。

ADR 0019 の専用旧 DB 移行を追加しない判断を覆す方針と、専用 backfill の廃止は存続する。任意の旧版からの upgrade、稼働 DB の追従、一般的な保存値補正、Claude Code / Cursor Agent の再解析は提供しない。通常 import の部分成功や full 再構築に移行条件を混ぜない。利用時の前提と旧 snapshot 形状は [移行手順](../docs/migration.md)、履歴保持制約は [要件](../docs/requirements.md) に置く。
