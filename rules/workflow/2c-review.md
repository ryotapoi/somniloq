# Step 2c: レビュー

動作確認を通過したら、`/review-code-all` を必ず実行する。

- `/review-code-all` は実装レビューの全チェーンを起動する。レビューはこのチェーンスキル経由で実行する（個別の `/review-code`, `/review-code-somniloq`, `/review-code-codex` 等は呼ばない）
- レビュー指摘があれば対応し、**再度 `/review-code-all` を通す**。コミットへ進むのはレビューを通してから
- コミット前のレビューは必須（スキップは禁止）

## レビューが完了したら

次のステップに進む: rules/workflow/3-finish.md を読む。
