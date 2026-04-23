# Backlog

## v0.3

### 背景（全タスク共通）

- 現状 `--short` は DB の `project_dir`（例: `-Users-name-Sources-org-my-repo`）から最後の `-` 以降を切り出して返している
- `project_dir` は Claude Code が生成するキーで、元パスの `/` を `-` に置換したもの。`/` 区切りと `-` 区切りの区別がつかない。リポジトリ名にハイフンを含むと一部だけが返る（期待: `my-repo` / 実際: `repo`）
- Claude Code は project dir 名生成時に `_` → `-` に変換している形跡もあり、`project_dir` から元パスの完全復元は不可能
- デフォルト表示（`--short` なし）は `project_dir` の生値で、読みにくい。`--short` の動機自体が「`project_dir` が読みにくい」の回避だった
- **解決**: JSONL の `cwd`（実パス）を使って本体リポジトリの絶対パス `repo_path` を解決し、DB に持たせる。デフォルト表示を `repo_path` に、`--short` を `repo_path` の basename に変更する

### 事前調査で判明している事実（再調査不要）

- `internal/core/jsonl.go:14` `RawRecord.CWD` は既に JSONL からパース済み
- `internal/core/import.go:140` で `SessionMeta.CWD` として upsert に渡っている
- `sessions` テーブルには**既に `cwd` カラムが存在**（`db.go:383` `schema`）
- `upsertSession` (`db.go:54`) は `COALESCE(NULLIF(excluded.cwd, ''), sessions.cwd)` で既存 cwd を上書きしない
- 実 DB（`~/.somniloq/somniloq.db`）の実測:
  - 既存セッションの大半は `cwd` に実パスを保持しており、欠損はごく少数（custom-title 専用のメタセッション等、JSONL に cwd が無いケース）
  - `project_dir` と `cwd` の対応例:
    - `-Users-name-Sources-org-my-repo` → `/Users/name/Sources/org/my-repo`
  - worktree の cwd は `/<repo>/.claude/worktrees/<name>` を含む
  - サブディレクトリで起動されたセッションでは `cwd` がリポジトリ root より深いこともある
  - 同一 `project_dir` に対して複数の `cwd` が紐づくケースあり（worktree・サブディレクトリ起動）
- → **DB 内バックフィルで完結できる（再 import 不要）**

### `repo_path` 解決ロジック（全タスクで共通の仕様）

入力は `cwd` 文字列。以下を上から試す:

1. `cwd` が空 → 空文字を返す
2. `cwd` が `/.claude/worktrees/` を含む → `/.claude/worktrees/` の直前までの絶対パスを返す（ディスクアクセス不要・削除済み worktree でも解ける。worktree 本体のリポジトリパス）
3. `git -C <cwd> rev-parse --show-toplevel` を実行し成功したら標準出力のパスを返す
4. どれも失敗 → 空文字

表示時の派生値 `repo_name` は `filepath.Base(repo_path)`。

### 表示仕様

- **デフォルト**（フラグなし）: `repo_path` を表示。`repo_path` が空なら従来の `project_dir` 生値にフォールバック
- **`--short`**: `repo_name`（`filepath.Base(repo_path)`）を表示。`repo_path` が空なら従来の「最後のハイフン要素」フォールバック

対象サブコマンド: `sessions` / `show` / `projects`

### リリース後のユーザー体験

新バイナリを入れて `somniloq import` を 1 回叩くだけ：

1. `OpenDB` 初回起動時にスキーママイグレーション（`ALTER TABLE sessions ADD COLUMN repo_path TEXT`）が自動で走る
2. `somniloq import` 実行時に、既存セッションの `cwd` を読んで `repo_path` を一括バックフィル（cwd 単位メモ化で数秒）
3. 同じ import 実行で通常の差分 import も走る（新規セッションは import 時に `repo_path` も保存）

以後、デフォルト出力が `repo_path` ベース、`--short` が `repo_name` ベースに変わる。

### タスク（上から順に実装・コミット。まとめて v0.3.0 としてリリース）

- [x] **`repo_path` 解決関数を追加（DB 変更なし）**
  - `internal/core/repo_path.go`（新規）に `ResolveRepoPath(cwd string) string` を実装
  - ロジックは上記「解決ロジック」の 1〜4
  - `git` 呼び出しは `os/exec` で `git -C <cwd> rev-parse --show-toplevel`。stderr 無視、非 0 終了は空扱い
  - `internal/core/repo_path_test.go`: ロジック 1/2/4 はユニットテストで網羅、3 はテスト用一時 git リポジトリで最低 1 ケース
  - この時点では誰も呼ばない（次コミットから使う）

- [ ] **スキーママイグレーション（`repo_path` カラム追加）**
  - `db.go` の `schema` に `repo_path TEXT` を追加（新規 DB 用）
  - `OpenDB` で既存 DB 向けに `PRAGMA table_info(sessions)` を見て `repo_path` が無ければ `ALTER TABLE sessions ADD COLUMN repo_path TEXT` を実行
  - `SessionMeta` に `RepoPath` フィールド追加、`upsertSession` の INSERT/UPDATE で扱う（`COALESCE(NULLIF(excluded.repo_path, ''), sessions.repo_path)`）
  - この時点では import 側は `RepoPath` を埋めないので全セッション NULL のまま。表示も変更なし
  - マイグレーションテスト: 既存スキーマの DB を開いて `repo_path` カラムが追加されることを確認

- [ ] **import 時に `repo_path` を解決して保存**
  - `import.go` の `processFile` で `SessionMeta` 組み立て時に `ResolveRepoPath(rec.CWD)` を呼んで `RepoPath` に入れる
  - 同一 `cwd` の連続行が多いので、`processFile` スコープで `map[string]string` でメモ化
  - テスト: JSONL に cwd を含むケースで `sessions.repo_path` が埋まることを確認

- [ ] **既存セッションのバックフィル**
  - `somniloq import` 実行時（`OpenDB` ではない）に、`repo_path` が NULL または空のセッションに対して、`cwd` から `ResolveRepoPath` で解決して UPDATE
  - 差分 import 本体の前段として走らせる（同一 import 実行内で既存分を埋めてから新規分を取り込む）
  - `cwd` 単位でメモ化
  - 解決不能（cwd 空・`.claude/worktrees/` 非該当・`git` 失敗）は空のまま。表示は従来フォールバックに落ちる
  - バックフィルは 1 回走れば十分だが、idempotent に書く（NULL/空だけ対象にすれば自然に達成）
  - テスト: `repo_path` NULL のセッションを用意してバックフィル後に埋まることを確認

- [ ] **表示ロジックを `repo_path` / `repo_name` ベースに切り替え、`--short` の意味を変更**
  - `cmd/somniloq` の表示ロジックを以下に変更:
    - デフォルト: `repo_path` が非空なら `repo_path`、空なら従来の `project_dir` 生値フォールバック
    - `--short`: `repo_path` が非空なら `filepath.Base(repo_path)`、空なら従来の「最後のハイフン要素」フォールバック
  - 対象サブコマンド: `sessions` / `projects` / `show`
  - `SessionRow` / `ProjectRow` に `RepoPath` を含め、`ListSessions` / `ListProjects` / `GetSession` の SELECT に `COALESCE(s.repo_path, '')` を追加
  - `projects` は **集計キーを `repo_path` に切り替える**:
    - `repo_path` が非空のセッションは `repo_path` でグルーピング
    - `repo_path` が空のセッションは従来どおり `project_dir` でグルーピング（フォールバック）
    - SQL 的には `GROUP BY COALESCE(NULLIF(repo_path, ''), project_dir)` 相当
    - 同一 `project_dir` に worktree やサブディレクトリ起動で複数の `cwd` がぶら下がっていても、`repo_path` は同じ値に解決されるので 1 行にまとまる
  - `--short` のヘルプ文を「shorten to repository basename」等に更新
  - テスト: `repo_path` が埋まっているセッション / 空のセッション双方でデフォルト・`--short` 出力を確認。`projects` では worktree セッションが本体リポジトリと同じ行に集約されることも確認
  - `rules/scope.md` の表示仕様説明を更新

- [ ] **ドキュメント更新**
  - `README.md` / `README.ja.md` のデフォルト出力・`--short` 説明を更新（breaking change を明記）
  - `references/knowledge.md` に「Claude Code の JSONL `cwd` は worktree でも実パスを保持している」「`project_dir` キーは `/` と `-` を区別できない」等の知見を追記
