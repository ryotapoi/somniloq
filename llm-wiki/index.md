---
regen: full
sources:
  - docs/rules/information-management.md
  - docs/rules/architecture.md
  - docs/rules/scope.md
  - docs/specs/jsonl-schema.md
  - llm-wiki/agent-workflow-map.md
  - llm-wiki/command-map.md
  - llm-wiki/configuration-and-projects.md
  - llm-wiki/display-and-turns.md
  - llm-wiki/import-pipeline.md
  - llm-wiki/sqlite-driver-notes.md
  - llm-wiki/storage-query-map.md
---

# llm-wiki

`llm-wiki/` は somniloq のコードを速く読むための AI 編纂の作業地図。各ページは「どこを読み・どこを直すか」の当たりをつけるための経路・注意点を置く場所であり、正本ではない。規範・仕様・設計理由は `docs/rules/` / `docs/specs/` / `docs/decisions/`、実装の事実はソースと tests が正本で、矛盾したらそちらに負ける。配置・情報分類・SSoT の正本は `docs/rules/information-management.md`。

各ページは次の 4 不変条件を満たす（点検は `wiki-lint` skill）:

- **速い**: ソース全追いより速く「どこを読み・何を直すか」の当たりがつく。経路・読む場所・注意点があり、単なる目次の羅列ではない。
- **docs レベルではない**: 正本（`docs/rules` / `docs/specs` / `docs/decisions`）・ソースを再掲しない。規範・仕様・設計理由はポインタ（パス・行・ADR 番号）で送る。
- **嘘がない**: 行番号・関数名・パス参照が現在のソースと一致している。
- **拾える**: この index.md から全ページに到達でき、ページ間リンクが GitHub で繋がる `[テキスト](相対パス.md)` 形式（`[[wikilink]]` は使わない）。

## 使い方

- 該当領域を触る作業の前に、関連ページを読んで読む順序・注意点の当たりをつける。
- ページ間リンクは `[テキスト](相対パス.md)` 形式で書く。`[[wikilink]]` は GitHub で繋がらないので使わない。
- `regen: full` / `compiled` ページは、frontmatter の `sources:` から再生成する前提なので手編集で本文を育てない。横断的な外部知見（`regen: none`）だけ手で育てる。
- 置き場所は `docs/rules/information-management.md` に従う。特定ソースに紐づく罠はそのコードのコメントへ、横断的な挙動・設計理解は該当地図へ。単一の集約知見ファイルは作らない。
- 正本と矛盾したら、地図側を直すか正本へ昇格する。地図に規範・仕様を抱え込ませない。

| ページ | regen | 内容 | 主なソース |
|---|---|---|---|
| [Command map](command-map.md) | full | CLI コマンドから入口関数・core クエリ・代表テストへ行く索引 | cmd/somniloq, internal/core |
| [Import pipeline](import-pipeline.md) | compiled | Claude Code / Codex JSONL が DB 行になるまでの読む順序 | internal/core/import.go, internal/ingest |
| [Storage and query map](storage-query-map.md) | compiled | schema, migration, query helper, backfill の変更入口 | internal/core/db*.go, internal/core/backfill.go |
| [Display and turns](display-and-turns.md) | compiled | show / outline / search の表示・ターン採番・TSV/JSON の導線 | cmd/somniloq, internal/core/db_query.go |
| [Configuration and projects](configuration-and-projects.md) | compiled | repo_path 解決、project alias、project filter の波及先 | cmd/somniloq/config.go, internal/core/repo_path.go |
| [Agent workflow map](agent-workflow-map.md) | compiled | Codex / Claude 側 workflow, skill, docs 配置の同期を見る導線 | AGENTS.md, CLAUDE.md, .agents, .claude |
| [SQLite driver notes](sqlite-driver-notes.md) | none | modernc.org/sqlite / SQLite の外部由来の罠 | internal/core/db.go, internal/core/db_schema.go, internal/core/db_query.go, internal/core/backfill.go |
