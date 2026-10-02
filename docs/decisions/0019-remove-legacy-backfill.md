# ADR 0019: 専用 backfill と旧 DB 移行の廃止

## Status

Accepted（2026-10-02 決定）

## Context

ADR 0003 は過去データの補正を import から分離した backfill として採用し、ADR 0004 は v0.3 から v0.4 への source 付き schema 移行をそのコマンドに集約した。実質利用者はプロジェクト所有者のみであり、公開機能として専用の旧 DB 救済経路を維持する必要がなくなった。

## Decision

公開 backfill と専用の旧データ補正・v0.3→v0.4 移行を廃止し、専用 upgrade 手段を追加しない判断とした。旧 DB 利用者のための代替コマンドや自動移行は、今回の利用者構成では保守負担に見合わないため採用しなかった。現行の提供範囲は `docs/rules/scope.md` に配布した。

ADR 0003 の専用補正コマンド採用と、ADR 0004 の専用 migration 採用を覆す。ADR 0004 の source、複合主キー・外部キーによる複数 source の schema 設計は存続する。OpenDB の一般 schema 管理と通常 import はこの判断の廃止対象ではない。
