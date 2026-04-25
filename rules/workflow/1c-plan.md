# Step 1-c: プラン作成・レビュー

## プラン作成

Plan モードでなければ EnterPlanMode する。調査・設計の結果をプランとして整理する。

## プランレビュー

プランの記述が完了したら、**ExitPlanMode を直接呼んではならない**。
必ず先に `/review-plan-all` スキルを Skill ツールで実行する。レビューを通るまで修正する。

レビュー完了後に ExitPlanMode を呼ぶ。
