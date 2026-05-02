# Backlog

## v0.3 — repo_path 設計のやり直し

- [x] `rules/scope.md` を新方針に更新（SSoT 先行）
- [x] `ResolveRepoPath` 手順 4 を「cwd 返却」に変更
- [x] import 時のメタ前置 INSERT を廃止し、会話レコードのみが session 行を作る
- [x] `projects` 集約と `--project` フィルタを `repo_path` 一本に絞る
- [x] `backfill` をデータ補正の単一窓口に拡張（`messages` なし行の DELETE + `repo_path` 補完 + 確認プロンプト）
- [x] `project_dir` カラムを完全撤去（書き込み・スキーマ・migration まで）
- [x] ドキュメント更新（README アップグレード手順、SKILL backfill 項、ADR 0003 update）
