# Step 2c: レビュー

## ICAR

- **Intent**: 動作確認を通過した実装に対して `/review-code-all` を回し、コミット前のレビューを必ず通す
- **Constraints**:
  - レビューは `/review-code-all` 経由で実行する（個別の `/review-code`, `/review-code-somniloq`, `/review-code-codex` 等は呼ばない）
  - 指摘があれば対応し、**再度 `/review-code-all` を通す**
  - **3 回目以降のレビューに入る前に、必ずユーザーに状況を報告して指示を仰ぐ**（周回数・残った指摘要旨・同根拠の再指摘有無を簡潔に伝え、選択肢は提示せず指示を待つ）
- **Acceptance**: 以下のどちらかを満たした状態（次は Step 3 コミット）
  - レビュー指摘 0 件
  - 残った指摘すべてが前回と**根拠（why）が同じ**再指摘（同じ角度。文面ではなく根拠で判定。A' の新しい角度なら対応してレビューへ戻る）
- **Relevant**: `/review-code-all`
