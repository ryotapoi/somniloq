# Scope

## 主要機能

### 取り込み（import）

- `~/.claude/projects/` を走査し、各 JSONL ファイルを列挙
- `import_state` と照合し、未取り込み or サイズ増加分を検出（差分取り込み）
- 各 JSONL を行単位で読み、`type` でフィルタ
- `user`/`assistant` → messages テーブルへ（text 部分のみ抽出）
- `custom-title`/`agent-name` → sessions テーブルの該当カラム更新
- `import_state` を更新
- `--full` フラグで全件再取り込み

### セッション一覧（sessions）

- セッション一覧を表示
- `--since` で期間フィルタ
- `--project` でプロジェクト名フィルタ

### 内容表示（show）

- セッション内容を Markdown で出力
- `--since` で直近の全セッションを一括表示
- `--format markdown` でフォーマット指定


## CLI インターフェース

```bash
cclog import                          # 全 JSONL を差分取り込み
cclog import --full                   # 全件再取り込み
cclog sessions                        # セッション一覧
cclog sessions --since 24h            # 直近24時間
cclog sessions --project Brimday      # プロジェクト名フィルタ
cclog show <session-id>               # セッション内容を Markdown で出力
cclog show --since 24h                # 直近24時間の全セッション
cclog --db /path/to/cclog.db ...      # DB パスの指定
```

## SQLite

- デフォルト配置: `~/.cclog/cclog.db`（`--db` フラグで変更可能）
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
