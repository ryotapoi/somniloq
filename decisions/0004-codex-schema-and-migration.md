# ADR 0004: Codex 対応のスキーマ設計と v0.3 → v0.4 migration 方針

## Status

Accepted

## Context

v0.4 で Codex（`~/.codex/sessions/` 配下の rollout JSONL）の取り込みに対応する。`rules/scope.md` で Claude Code と Codex を共通の `sessions` / `messages` テーブルに正規化する方針は決まったが、以下の設計判断が未確定だった。

1. `sessions` / `messages` の主キーをどうするか（Claude Code の session_id と Codex の session_id が衝突しうる前提でどう一意性を取るか）
2. `import_state` の主キーをどうするか（差分取り込みの単位）
3. v0.3 → v0.4 の既存データ移行をどう実施するか
4. SQLite で主キー変更を伴うスキーマ変更をどう実装するか

## Considered Options

### 1. sessions / messages の主キー

- **複合主キー `(source, session_id)`**: Claude Code は UUID、Codex は rollout 内で一意の ID。両者を同じテーブルに入れると将来的に衝突しうるため、source を主キーに含めて分離する。messages 側も `(source, session_id)` の複合外部キーになる
- **session_id 単独主キー + プレフィックス付与**: import 時に `claude_code:<uuid>` のような形に変換して単一カラムに収める。スキーマは単純だが、表示・検索・他システム連携で「元の session_id」を切り出す必要が出る

### 2. import_state の主キー

- **`jsonl_path` 単独主キー + `source` カラム追加**: Claude Code は `~/.claude/projects/...`、Codex は `~/.codex/sessions/...` でパス空間が完全に分離しているため、絶対パスだけで一意。source は補助情報として保持
- **`(source, jsonl_path)` 複合主キー**: sessions と揃えて複合化する。理論的には堅牢だが、パス衝突は実際には起きないため一意性に貢献しない
- **source 別テーブル分割**（`claude_code_import_state` / `codex_import_state`）: 完全独立。スキーマが分かれることで共通コードに分岐が増える

### 3. v0.3 → v0.4 の既存データ移行

- **`OpenDB` 内で自動 migration**: 起動時に PRAGMA table_info で source カラムの有無を確認し、無ければ追加 + 既存行に `'claude_code'` を埋め込む。ユーザーは何もしなくていい
- **`backfill` コマンドに統合**: 既存の `somniloq backfill`（v0.2.x 由来データの補正窓口）に、source カラム追加・既存行への埋め込みも含める。ユーザーが v0.4 起動後に明示的に 1 回叩く
- **独立 migration コマンド**（例: `somniloq migrate`）: 専用サブコマンドを新設。明示性は高いが、CLI の窓口が増える

### 4. 主キー変更の実装方式

- **テーブル再作成方式**（CREATE new + INSERT SELECT + DROP + RENAME）: SQLite で主キー構造を変える標準手順。messages の外部キーも同時に張り直す。トランザクション内で実施
- **UNIQUE INDEX で代用**: 既存 `session_id` 主キーは維持し、`(source, session_id)` には UNIQUE INDEX を張るだけ。手順は軽いが、scope.md の「PRIMARY KEY (source, session_id)」と乖離する

## Decision

以下を v0.4 のスキーマ設計と migration 方針として確定する。

### sessions / messages

- `sessions` の主キーは **`(source, session_id)` 複合主キー**
- `messages` に `source` / `session_id` カラムを追加し、外部キーは **`(source, session_id) REFERENCES sessions(source, session_id)`** の複合外部キー
- `messages.uuid` は引き続きグローバル一意な PRIMARY KEY として維持（Claude Code 由来は UUID、Codex 由来は `(source, rollout_path, line_number)` 等から決定的に導出する想定。具体的な導出方式は adapter 実装タスクで決める）

### import_state

- 主キーは **`jsonl_path` 単独**
- `source` カラムを追加し、補助情報（どの adapter が取り込んだかの記録）として保持
- 理由: Claude Code と Codex はファイル配置のベースディレクトリが完全に分離していて衝突しない。複合主キー化は理論的な堅牢性だけで実利が薄く、migration コストを正当化できない

### v0.3 → v0.4 migration

- 既存 `backfill` コマンドに **「source カラム追加・既存 sessions/messages 行に `'claude_code'` を埋め込み・主キー再構築」** のステップを追加する
- `OpenDB` 起動時の自動 migration は採用しない。理由:
  - 主キー再構築（テーブル再作成）はリスクが高く、ユーザーが意図しないタイミングで走るのは避けたい
  - v0.3 でも backfill は明示実行の建付けで、その流儀を維持する（ADR 0003 の方針と整合）
- backfill のドキュメント（README / scope.md / -h ヘルプ）に「v0.3 → v0.4 アップグレード後に 1 回実行」と記載する

### 主キー変更の実装方式

- **テーブル再作成方式** を採用
- 単一トランザクション内で `sessions_new` を作成 → `INSERT INTO sessions_new SELECT 'claude_code', ... FROM sessions` → `DROP TABLE sessions` → `ALTER TABLE sessions_new RENAME TO sessions`、を `messages` についても同様に実施
- 既存 ADR 0003 の `Backfill` API（`(BackfillResult, error)` 戻り値）を踏襲し、`BackfillResult` にカウンタフィールドを追加する形で拡張する

## Consequences

- スキーマ migration が backfill 経由に集約され、起動時の挙動は v0.3 と同じ（重い ALTER が暗黙には走らない）
- v0.3 → v0.4 アップグレード後、ユーザーは `somniloq backfill` を 1 回叩く必要がある。叩かないと v0.4 の `import` / `import-codex` は失敗する想定（source カラムが無いので ON CONFLICT 等が破綻する）。backfill が必須であることをドキュメントに明示する
- `import_state` の主キーが単純なため、共通の差分取り込みコード（`UpsertImportState` / `GetImportState`）は source を意識せずに動かせる
- `messages` の主キーが `uuid` 単独のままなので、Codex 側で UUID に相当するものを決定的に生成する責務が adapter 実装タスクに残る（`(source, rollout_path, line_number)` ハッシュ等）
- 後続タスク（adapter 実装、CLI `import-codex` 追加）は本 ADR のスキーマ前提で進める

## References

- `rules/scope.md` のテーブル設計セクション
- `decisions/0003-backfill-as-separate-subcommand.md`（backfill コマンドの位置付け）
- `references/knowledge.md`（modernc.org/sqlite の罠、PRAGMA check-first migration パターン）
- `backlog/backlog.md` v0.4 セクション
