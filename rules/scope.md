# Scope

## 主要機能

### 取り込み（import）

- `~/.claude/projects/` を走査し、各 JSONL ファイルを列挙
- `import_state` と照合し、未取り込み or サイズ増加分を検出（差分取り込み）
- 各 JSONL を行単位で読み、`type` でフィルタ
- `user`/`assistant` → messages テーブルへ（text 部分のみ抽出）
- `custom-title`/`agent-name` → sessions テーブルの該当カラム更新
- `import_state` を更新
- `--full` フラグで全件再取り込み（確認プロンプトあり、デフォルト No）
  - `--yes` で確認をスキップ
  - 非対話環境（パイプ、CI 等）では `--yes` が必須

### セッション一覧（sessions）

- セッション一覧を表示
- `--since`/`--until` で時刻フィルタ（相対: `24h`, `7d`、絶対: `2026-03-28`, `2026-03-28T15:00`）。絶対日付はローカルタイム。出力のタイムスタンプもローカルタイム（`2006-01-02 15:04` 形式）
- 時刻は `started_at ~ ended_at` の範囲形式で表示。ended_at がない場合は `started_at ~`
- `--project` でプロジェクト名フィルタ
- デフォルト表示では worktree サフィックス（`--claude-worktrees-*`）を除去して正規化
- `--short` で `project_dir` の最後のハイフン区切り要素のみに短縮表示

### プロジェクト一覧（projects）

- プロジェクト一覧をセッション数とともに表示
- `--since`/`--until` で時刻フィルタ（`started_at` 基準、sessions と同じ）
- worktree セッションはルートプロジェクトにマージし、セッション数を合算
- `--short` で `project_dir` の最後のハイフン区切り要素のみに短縮表示
- ソート: 直近セッション開始順（降順）

### 内容表示（show）

- セッション内容を Markdown で出力
- Started 行に `started_at ~ ended_at` の時刻範囲を表示。ended_at がない場合は `started_at ~`
- `--since`/`--until` で期間指定して一括表示
- `--summary` で各セッションの最初のユーザーメッセージのみ表示
- デフォルト表示では worktree サフィックスを除去して正規化
- `--short` で `project_dir` の最後のハイフン区切り要素のみに短縮表示
- `--format markdown` でフォーマット指定


## CLI インターフェース

```bash
somniloq import                          # 全 JSONL を差分取り込み
somniloq import --full                   # 全件再取り込み（確認あり）
somniloq import --full --yes             # 確認なしで全件再取り込み
somniloq sessions                        # セッション一覧
somniloq sessions --since 24h            # 直近24時間
somniloq sessions --since 2026-03-28     # 3/28 以降
somniloq sessions --until 2026-03-28     # 3/28 終わりまで
somniloq sessions --since 7d --until 2h  # 直近7日間から最新2時間を除外
somniloq sessions --project Brimday      # プロジェクト名フィルタ
somniloq sessions --short                # プロジェクト名を短縮表示
somniloq show <session-id>               # セッション内容を Markdown で出力
somniloq show --since 24h                # 直近24時間の全セッション
somniloq show --since 2026-03-28 --until 2026-03-29  # 3/28 の全セッション
somniloq show --summary --since 24h                  # 直近24時間の各セッションの冒頭のみ
somniloq show --since 24h --short                    # プロジェクト名を短縮表示
somniloq projects                        # プロジェクト一覧
somniloq projects --short                # プロジェクト名を短縮表示
somniloq projects --since 7d             # 直近7日間にセッションがあるプロジェクト
somniloq --db /path/to/somniloq.db ...      # DB パスの指定
somniloq --version                          # バージョン表示
```

## SQLite

- デフォルト配置: `~/.somniloq/somniloq.db`（`--db` フラグで変更可能）
- セッション横断で使うため、特定プロジェクトの中には置かない

### テーブル設計

```sql
-- セッション単位のメタデータ
CREATE TABLE sessions (
    session_id TEXT PRIMARY KEY,  -- UUID
    project_dir TEXT NOT NULL,    -- プロジェクトディレクトリ名
    cwd TEXT,                     -- 作業ディレクトリ
    git_branch TEXT,
    custom_title TEXT,            -- custom-title レコードから
    agent_name TEXT,              -- agent-name レコードから
    version TEXT,                 -- Claude Code バージョン
    started_at TEXT,              -- 最初のレコードの timestamp
    ended_at TEXT,                -- 最後のレコードの timestamp
    imported_at TEXT NOT NULL     -- 取り込み日時
);

-- 会話ターン（user/assistant の text のみ）
CREATE TABLE messages (
    uuid TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(session_id),
    parent_uuid TEXT,
    role TEXT NOT NULL,           -- 'user' or 'assistant'
    content TEXT NOT NULL,        -- text 部分のみ結合した文字列
    timestamp TEXT NOT NULL,
    is_sidechain BOOLEAN DEFAULT FALSE,
    UNIQUE(uuid)
);

-- 取り込み状態の追跡
CREATE TABLE import_state (
    jsonl_path TEXT PRIMARY KEY,  -- JSONL ファイルのパス
    file_size INTEGER,            -- 最終取り込み時のファイルサイズ
    last_offset INTEGER,          -- 最終取り込み行のバイトオフセット
    imported_at TEXT NOT NULL
);
```

## スキーマ変更への対応方針

- Go の struct タグで既知フィールドのみデコードし、未知は無視（デフォルト挙動）
- `version` フィールドを保存しておけば、問題発生時にバージョン別の切り分けが可能
