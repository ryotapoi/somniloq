# Maintenance (L3 Review)

## ICAR

- **Intent**: 単一タスクの範囲を超えて、構造・負債・重複・テスト戦略を棚卸しし、必要な改善タスクを作る。
- **Constraints**:
  - タスク内ではなく、節目で呼ぶ。タスク完了の度に呼ぶものではない。
  - 今回の差分ではなく、今後の変更コストを下げる観点で見る。
  - すぐ直すものと backlog に積むものを分ける。
  - 改善タスクは 1 commit に収まる粒度にする。
  - 仕様や設計方針の変更が必要なら `docs/decisions/` または `docs/rules/` 更新を検討する。
- **Acceptance**:
  - 構造上の問題、リファクタ候補、テスト戦略の不足が整理されている。
  - 必要な改善が `backlog/backlog.md` に追跡可能な形で入っている。
  - すぐ着手する改善と先送りする改善が分かれている。
- **Relevant**:
  - 最近の git history
  - `backlog/backlog.md`
  - 変更が多かったモジュール（`internal/core/` 配下、`cmd/somniloq/` 等）
  - `docs/rules/architecture.md`
  - `llm-wiki/`

## Use When

- 複数コミットやマイルストーン（v0.3 など）の区切り
- 同じ種類の修正が続いている
- 実装中やレビューでリファクタ候補が複数出た
- 久々に広い領域を触った
- review で同種の指摘が繰り返された

## Tools

- 棚卸し・健康診断: `maintenance-audit` スキル（軽い整合性・負債・backlog 鮮度の light pass から、テスト・カバレッジ・行数・依存方向・凝集度・分割の deep pass まで、scope で深さを指定）
- module / 配置 / 依存方向の境界判断: `module-boundary` スキル
- llm-wiki 健全性点検: `wiki-lint` スキル（孤立・リンク切れ・sources 切れの機械検証に加え、「速い / docs レベルでない / 嘘がない / 拾える」の不変条件を照合する）

## Flow ICAR

### Audit

- **Intent**: 節目の状態を棚卸しし、今後の変更コストを下げる改善候補を切り出す。
- **Constraints**:
  - 最近の git history、backlog、変更が多かったモジュール、`docs/rules/architecture.md`、`llm-wiki/` を必要範囲で確認する。
  - すぐ直す改善と backlog に積む改善を分ける。
  - `wiki-lint` を使う場合は、機械検証だけでなく llm-wiki の不変条件も照合する。
- **Acceptance**: 改善候補の扱いが、今すぐ着手・backlog・対応不要に分かれている。
- **Relevant**: 最近の git history、`backlog/backlog.md`、変更が多かったモジュール、`docs/rules/architecture.md`、`llm-wiki/`。

## Stop Conditions

- 改善が大きすぎて複数タスクに分割すべき
- プロダクト方針やアーキテクチャ方針の判断が必要
