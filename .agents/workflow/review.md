# Review Workflow

## ICAR

- **Intent**: 完了前に、差分が要求・仕様・既存設計を壊していないことを確認する。
- **Constraints**:
  - 粗探しではなく、実害・仕様逸脱・テスト不足・設計劣化を見る。
  - 小さい変更は self-check でよい。
  - SQLite / migration / JSONL import / CLI 互換性 / アーキテクチャ境界などは、該当 skill や別視点レビューを使う。
  - 指摘に対応しない場合は理由を残す。
- **Acceptance**:
  - 選んだレビュー深度と理由が説明できる。
  - 指摘があれば対応済み、または対応しない理由が明確。
  - レビュー後に変更した場合、必要な再検証が済んでいる。
- **Relevant**:
  - 変更差分
  - plan または要求
  - 検証結果
  - 関連する `rules/`, `specs/`, `references/knowledge.md`

## Depth

- **Self-check**: Small 変更。main で `git diff` を読み、要求と検証結果を照合する。
- **Targeted**: 領域固有リスクがある変更。`somniloq-risk-check`, `design-decision` など該当観点で確認する。
- **External**: 大きい、曖昧、High-risk、または設計判断が重い変更。`change-review` などの別視点を入れる。
- **Maintenance**: 今回の差分ではなく、複数タスク後の全体構造・負債を見る。`maintenance.md` を使う。

## somniloq Review Triggers

- SQLite schema / migration / `backfill` / DELETE を伴う処理
- SQL 集約、プレースホルダ、トランザクション、`modernc.org/sqlite` 固有挙動
- Claude Code / Codex JSONL import、source 差異、schema drift、差分取り込み
- CLI stdout/stderr、TSV/Markdown、exit code、確認プロンプト
- `cmd/somniloq` と `internal/core` の責務境界
- README / rules / references と実装の同期

## Stop Conditions

- 指摘対応が仕様・CLI 挙動・設計方針を変える。
- 必要な別視点レビューが実行できない。
