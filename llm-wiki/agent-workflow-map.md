---
regen: compiled
sources:
  - AGENTS.md
  - CLAUDE.md
  - .agents/workflow/goal.md
  - .agents/workflow/change/workflow.md
  - .agents/workflow/change/implement.md
  - .agents/workflow/change/review.md
  - .claude/workflow/goal.md
  - .claude/workflow/change/workflow.md
  - .claude/workflow/change/review.md
  - .claude/rules/docs.md
  - docs/rules/information-management.md
---

# Agent workflow map

Agent 向けファイルを触るときの同期地図。正本は `AGENTS.md` / `CLAUDE.md` / `.agents/` / `.claude/` の各 workflow で、このページは読む順序だけをまとめる。

## 入口

- Codex: `AGENTS.md` -> `.agents/workflow/change/workflow.md` -> phase workflow。
- Claude Code: `CLAUDE.md` -> `.claude/workflow/change/workflow.md` -> phase workflow。
- Goal: Codex は global `goal-workflow` skill から `.agents/workflow/goal.md`、Claude は `.claude/workflow/goal.md`。

## 同期が必要な変更

- 情報配置を変える: `docs/rules/information-management.md` -> `AGENTS.md` / `CLAUDE.md` -> `.agents/workflow/*` / `.claude/workflow/*` -> commit skill。
- review depth を変える: `.agents/workflow/change/review.md`, `.claude/workflow/change/review.md`, 関連 review skill。
- docs / llm-wiki 運用を変える: `.claude/rules/docs.md` と `AGENTS.md` の対応箇所を見る。Codex 側には path-scoped rule ファイルの仕組みがないため、方向性の parity を確認する。

## 変更時の注意

- `.claude/` と `.agents/` は逐語一致ではなく、各エージェントの tool / subagent 仕組みに合わせる。
- ただし目的・制約・判断基準は揃える。片側だけ旧パスや旧知見方針を残さない。
