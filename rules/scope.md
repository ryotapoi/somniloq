# Scope

## 主要機能

### 取り込み（import）

- `~/.claude/projects/` を走査し、各 JSONL ファイルを列挙
- `import_state` と照合し、未取り込み or サイズ増加分を検出（差分取り込み）
- 各 JSONL を行単位で読み、`type` でフィルタ
- `user`/`assistant` → messages テーブルへ（text 部分のみ抽出）
- `user`/`assistant` レコードが初出のときだけ `sessions` 行を作成する（`messages` 0 件で残るケースの扱いはバックフィル節参照）
- メタセッション（`custom-title` / `agent-name` 単独で `user`/`assistant` を持たない）は DB に保存しない。当該ファイルの `import_state` も進めず、後で会話レコードが追記されたときに先頭から再読み込みできる状態を維持する
- `user`/`assistant` の `cwd` から `repo_path` を解決して sessions に保存。`cwd` は会話レコードでは通常非空のため、会話セッションでは `repo_path` も通常非空（`ResolveRepoPath` 手順 4 で `cwd` 自体を返すため、`cwd` 非空なら必ず解決される）
- `custom-title` / `agent-name` レコードは、ファイル走査終了時点で対応する `sessions` 行が存在するときのみ反映する
- `import_state` を更新
- `--full` フラグで全件再取り込み（確認プロンプトあり、デフォルト No）
  - `--yes` で確認をスキップ
  - 非対話環境（パイプ、CI 等）では `--yes` が必須

### バックフィル（backfill）

v0.2.x 由来データの補正窓口。以下を順に実行する。

- `messages` を持たない `sessions` 行を DELETE
  - 主目的は v0.2.x 由来のメタ前置 INSERT 残骸の除去
  - 副次的に、text 抽出結果が空の `user`/`assistant` レコードしか持たないセッション（`tool_use` のみ・添付のみ・空白のみ）も消える。取り込み側は text 非空判定の前に `upsertSession` を呼ぶため `messages` 0 件で残る仕様で、show / sessions 一覧で実体が無く実害はほぼゼロ。`--full` で再取り込みすれば戻る
- `repo_path IS NULL` かつ `cwd` 非空 の行を `ResolveRepoPath` で埋める（手順 4 が cwd 返却になったため `cwd` 非空なら必ず解決される）
- DELETE 対象が 1 件以上ある場合のみ件数を起動時に表示し確認プロンプトを出す（デフォルト No）。0 件なら無確認で進む。`--yes` で確認をスキップ。非対話環境（パイプ・CI 等）では DELETE 対象 1 件以上のとき `--yes` 必須（`import --full` と同じ作法）
- `import` から独立。v0.3 へアップグレード後に一度叩く想定

### セッション一覧（sessions）

- セッション一覧を表示
- `--since`/`--until` で時刻フィルタ（相対: `24h`, `7d`、絶対: `2026-03-28`, `2026-03-28T15:00`）。絶対日付はローカルタイム。出力のタイムスタンプもローカルタイム（`2006-01-02 15:04` 形式）
- 時刻は `started_at ~ ended_at` の範囲形式で表示。ended_at がない場合は `started_at ~`
- `--project` は `repo_path` への substring マッチ。`project_dir` は対象としない（LIKE のターゲットは `repo_path` のみ。LIKE メタ文字の扱いは Known limitations 参照）
- `repo_path` は絶対パスのため、`/` セグメントを跨いだ部分一致（例: `--project Sources/ryot`）も可能
- デフォルト表示は `repo_path` をそのまま
- `--short` は `filepath.Base(repo_path)`（ハイフン保持）

### プロジェクト一覧（projects）

- プロジェクト一覧をセッション数とともに表示
- `--since`/`--until` で時刻フィルタ（`started_at` 基準、sessions と同じ）
- 集約キーは `repo_path` 一本。worktree とサブディレクトリ起動は SQL 側で本体リポジトリの行に集約される（cmd 層での後段マージは行わない）
- 出力 1 列目は `repo_path` そのもの
- `--short` で `filepath.Base(repo_path)`
- ソート: 直近セッション開始順（降順）

### 内容表示（show）

- セッション内容を Markdown で出力
- Started 行に `started_at ~ ended_at` の時刻範囲を表示。ended_at がない場合は `started_at ~`
- `--since`/`--until` で期間指定して一括表示
- `--summary N` で各セッションの user メッセージ先頭 N 件を表示（`/clear` と `<local-command-caveat>` はスキップ）。`0` または未指定で従来の全文表示
- `--include-clear` で `/clear`・caveat のスキップを無効化（`--summary >= 1` が前提）
- メタデータ `Project` 行は `repo_path` をそのまま表示
- `--short` で `filepath.Base(repo_path)`
- `--project` は sessions と同じフィルタ規則（`repo_path` 単独 substring。`project_dir` は対象外）
- `--format markdown` でフォーマット指定


## CLI インターフェース

```bash
somniloq import                          # 全 JSONL を差分取り込み
somniloq import --full                   # 全件再取り込み（確認あり）
somniloq import --full --yes             # 確認なしで全件再取り込み
somniloq backfill                        # 既存セッションの補正（DELETE 対象があれば確認）
somniloq backfill --yes                  # 確認なしで補正
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
somniloq show --summary 1 --since 24h                # 直近24時間の各セッションの冒頭 1 件
somniloq show --summary 3 --since 24h                # 冒頭 3 件
somniloq show --summary 1 --include-clear --since 24h  # /clear・caveat もスキップせずに表示
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
    project_dir TEXT NOT NULL,    -- 取り込み時の書き込みは継続するが、v0.3 完成時点ではクエリ側（フィルタ・集約・表示）で使わない。JSONL ファイルパス由来の出自情報として保持。撤去はリリース後に再評価（backlog 参照）
    cwd TEXT,                     -- 作業ディレクトリ。会話レコードでは通常非空
    repo_path TEXT,               -- ResolveRepoPath（internal/core/repo_path.go）で解決したリポジトリパス。会話セッションでは通常非空
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

## Known limitations

### 移行期限定（v0.2.x → v0.3）

データ補正完了が一般化したら本書から削除する。

- 過去に `repo_path NULL` のまま取り込まれた v0.2.x 由来セッションが DB に残っている（= `somniloq backfill` を未実行）状態だと、以下の影響が出る:
  - `projects` 集約で `repo_path` キーが空のグループに、複数の異なるリポジトリの NULL セッションがまとめて潰れて 1 行表示される（`GROUP BY repo_path` 一本のため）
  - `sessions` / `projects` / `show` の通常表示で「Project 列が空欄になる」（フォールバック削除によるストレートな退行）
  - `--short` 表示も空のままになり得る
  - `sessions --project <repo>` および `show --project <repo>` フィルタでヒットしない（`repo_path IS NULL` の行は LIKE にマッチしないため。旧仕様では `project_dir` 経由で引けていた）
  - `somniloq backfill` 実行で `repo_path` 補完と `messages` を持たない残骸の DELETE が同時に走り、上記すべて解消する

### 恒久的な制約

- Claude Code が将来 `cwd` 空の `user`/`assistant` レコードを生成する仕様になった場合、somniloq 側ではそのまま `repo_path` 空で保存する。`projects` 集約で「複数リポジトリが空グループに潰れる」上記の問題と同根。その時点で対応方針を再検討する
- `--project` の値は SQLite LIKE のメタ文字（`%`、`_`）を素通しでクエリに渡す（既存挙動の継承）。例: `--project my_repo` は `_` が 1 文字ワイルドカードとして解釈されるため `myXrepo` のような値にも誤マッチする可能性がある

## スキーマ変更への対応方針

- Go の struct タグで既知フィールドのみデコードし、未知は無視（デフォルト挙動）
- `version` フィールドを保存しておけば、問題発生時にバージョン別の切り分けが可能
