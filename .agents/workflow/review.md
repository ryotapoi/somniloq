# Review Workflow

## ICAR

- **Intent**: 完了前に、差分が要求・仕様・既存設計を壊していないことを確認する。
- **Constraints**:
  - 粗探しではなく、実害・仕様逸脱・テスト不足・設計劣化を見る。
  - 小さい変更は self-check でよい。
  - SQLite / migration / JSONL import / CLI 互換性 / アーキテクチャ境界などは、該当 skill や別視点レビューを使う。
  - テスト可能な振る舞い変更や bug fix に unit / regression test がない場合は、原則 blocker として扱う（理由がある例外のみ許容）。
  - レビュー周回は最大 3 周。3 周で収束しなければそれ以上回さず打ち切る。打ち切った場合は残った指摘と周回数を記録し、タスク完了報告（Goal なら Goal 完了報告）で `レビュー上限超過` として通知する。
  - 指摘に対応しない場合は理由を残す。
- **Acceptance**:
  - 選んだレビュー深度と理由が説明できる。
  - 指摘があれば対応済み、または対応しない理由が明確。
  - レビュー後に変更した場合、必要な再検証が済んでいる。
- **Relevant**:
  - 変更差分
  - plan または要求
  - 検証結果
  - 関連する `docs/rules/`, `docs/specs/`, `llm-wiki/`

## Depth

- **Self-check**: Small 変更。main で `git diff` を読み、要求と検証結果を照合する。
- **Standard**: Small 以外の実装差分。`change-review` を通し、指摘を採否判断して反映する。
- **Targeted**: 領域固有リスクがある変更。Standard に加えて `project-risk-check` で確認する。
- **External**: 大きい、曖昧、High-risk、または設計判断が重い変更。Standard に加えて別系統レビューを入れる（Goal では `goal.md` の Claude review。単発で Claude review が必要ならユーザーに確認してから `claude-review-request` を使う）。
- **Structural**: 構造劣化リスク（巨大化、分岐増加、責務境界の濁り、薄い抽象化、型境界の曖昧さ）がある変更。Standard に加えて `thermo-nuclear-code-quality-review` を必須で使う。
- **Maintenance**: 今回の差分ではなく、複数タスク後の全体構造・負債を見る。`maintenance.md` を使う。

Goal 全体の commit range に対する Claude review は、各 commit のここでの review とは別に Goal 完了条件として `goal.md` の Claude Review で実施する。

## somniloq Review Triggers

- SQLite schema / migration / `backfill` / DELETE を伴う処理
- SQL 集約、プレースホルダ、トランザクション、`modernc.org/sqlite` 固有挙動
- Claude Code / Codex JSONL import、source 差異、schema drift、差分取り込み
- CLI stdout/stderr、TSV/Markdown、exit code、確認プロンプト
- `cmd/somniloq` と `internal/core` の責務境界
- README / docs / llm-wiki と実装の同期

## Stop Conditions

- 指摘対応が仕様・CLI 挙動・設計方針を変える。
- 必要な別視点レビューが実行できない。
