# Scope

本書が CLI 仕様・コマンド挙動・スキーマの正。README.md / README.ja.md は本書の派生ビューなので、本書のこれらの記述を変更したら README 両方を同期する。

## 主要機能

### 取り込み（import）

source（DB 内部値は `claude_code` / `codex`）ごとに専用の adapter で取り込む。共通の正規化スキーマ（`sessions` / `messages`）に保存する点は両者で一致するが、ファイル配置・レコード形式・差分検出キーは source ごとに異なる。

`somniloq import` はデフォルトで Claude Code と Codex の両方を同じ SQLite DB に取り込む。対象を絞る場合は CLI 表記の `--source all|claude-code|codex` を使う。

#### エラー処理と取り込みサマリ（source 共通）

- 取り込み終了時に `Imported <n> files (<scanned> scanned, <skipped> skipped, <failed> failed, <unparsed> unparsed lines)` を stdout に出力する
- parse / 正規化できない行（壊れた JSON、不正な payload）はスキップして続行し、`unparsed lines` に計上する。source が意図的に無視するレコード型（未知 type・非 message レコード・空行等）はカウントしない（`docs/decisions/0009-unparsed-line-visibility.md`）
- ディレクトリ走査で読めないディレクトリはスキップして続行し、発見できたファイルは取り込む。ファイル単位の取り込み失敗も同様にスキップして続行する（`docs/decisions/0010-non-fatal-scan-errors.md`）
- スキップしたエラーは stderr に列挙し、1 件以上あれば exit code は 1（取り込み自体は部分的に完了している）
- source のルートディレクトリが存在しない場合は、その source を未使用として扱いエラーにしない

#### Claude Code 用（`somniloq import --source claude-code`）

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
  - `--source` 指定時も DB 全体を削除し、指定 source だけを再取り込みする

#### Codex 用（`somniloq import --source codex`）

- `~/.codex/sessions/` 配下の日付ディレクトリを再帰走査し、rollout JSONL を列挙
- 各 JSONL を行単位で読み、`response_item` かつ `payload.type == "message"` かつ `role in ("user", "assistant")` のレコードのみを取り込み対象とする
- `payload.content` は `input_text` / `output_text` / `text` block の `text` のみを抽出し、複数 block は空行区切りで結合する
- `session_id` は `session_meta.payload.id` を使う。ファイル名 stem は走査時の補助 ID に留める
- `session_meta.payload.cwd` から `repo_path` を解決して sessions に保存（解決ロジックは Claude Code 側と共有）
- `git_branch` は `session_meta.payload.git.branch`、`version` は `session_meta.payload.cli_version` から保存する
- `messages.uuid` の一意性は `(rollout_path, line_number)` ベースで判定（Codex のレコードは Claude Code のような UUID を持たないため）
- 差分取り込みで追記分だけを読む場合も、offset 直前までの `session_meta` を先に読み直して session メタデータを復元する
- 差分取り込み・`--full` 等のオプション体系は `import` と揃える

### バックフィル（backfill）

過去バージョン由来のデータ補正と、メジャーバージョンアップ時のスキーマ移行の窓口。以下を順に実行する。

- v0.4 スキーマ移行（v0.3 由来 DB のみ実行。実行済みなら no-op）:
  - `sessions` / `messages` に `source` カラムを追加し、既存行に `'claude_code'` を埋め込む
  - `sessions` の主キーを `(source, session_id)` 複合主キーに、`messages` の外部キーを `(source, session_id)` 複合外部キーに張り直す（テーブル再作成方式）
  - `import_state` に `source` カラムを追加し、既存行に `'claude_code'` を埋め込む
  - 詳細は `docs/decisions/0004-codex-schema-and-migration.md` 参照
- `messages` を持たない `sessions` 行を DELETE
  - 主目的は v0.2.x 由来のメタ前置 INSERT 残骸の除去
  - 副次的に、text 抽出結果が空の `user`/`assistant` レコードしか持たないセッション（`tool_use` のみ・添付のみ・空白のみ）も消える。取り込み側は text 非空判定の前に `upsertSession` を呼ぶため `messages` 0 件で残る仕様で、show / sessions 一覧で実体が無く実害はほぼゼロ。`--full` で再取り込みすれば戻る
- `repo_path IS NULL` かつ `cwd` 非空 の行を `ResolveRepoPath` で埋める（手順 4 が cwd 返却になったため `cwd` 非空なら必ず解決される）
- DELETE 対象が 1 件以上ある場合のみ件数を起動時に表示し確認プロンプトを出す（デフォルト No）。0 件なら無確認で進む。`--yes` で確認をスキップ。非対話環境（パイプ・CI 等）では DELETE 対象 1 件以上のとき `--yes` 必須（`import --full` と同じ作法）
- `import` から独立。v0.3 / v0.4 へアップグレード後に一度叩く想定（v0.4 ではスキーマ移行が含まれるため、`import` を叩く前の実行が必須）

### セッション一覧（sessions）

- セッション一覧を表示
- `--since`/`--until` で時刻フィルタ（相対: `24h`, `7d`、絶対: `2026-03-28`, `2026-03-28T15:00`）。絶対日付はローカルタイム。出力のタイムスタンプもローカルタイム（`2006-01-02 15:04` 形式）
- 時刻は `started_at ~ ended_at` の範囲形式で表示。ended_at がない場合は `started_at ~`
- `--project` は `repo_path` への substring マッチ（LIKE メタ文字の扱いは Known limitations 参照）。値が config の alias グループに完全一致する場合はグループ全名に展開する（「設定ファイル」節参照）
- `repo_path` は絶対パスのため、`/` セグメントを跨いだ部分一致（例: `--project Sources/ryot`）も可能
- 表示は config の `projectAliases` に一致する場合は canonical 名のみ。一致しない場合、デフォルト表示は `repo_path` をそのまま
- `--short` は alias 非一致時に `filepath.Base(repo_path)`（ハイフン保持）
- 出力 TSV の列: `session_id`, `started_at ~ ended_at`, `project`, `custom_title`, `message_count`, `body_size`
- `body_size` は非 sidechain メッセージの本文合計サイズ（UTF-8 バイト数）。show が出力する量の予測値として使う（show 前に大きいセッションかを判定する用途）。文字数でなくバイト数なのは、コンテキスト量の感覚と一致させるため。`message_count` は従来どおり sidechain を含む全行数
- `--format tsv|json`（デフォルト `tsv`）。JSON のフィールドは `source`, `sessionId`, `project`, `title`, `startedAt`, `endedAt`, `messageCount`, `bodySize`（共通仕様は「JSON 出力」節参照）

### プロジェクト一覧（projects）

- プロジェクト一覧をセッション数とともに表示
- `--since`/`--until` で時刻フィルタ（`started_at` 基準、sessions と同じ）
- SQL 側の集約キーは `repo_path` 一本。worktree とサブディレクトリ起動は SQL 側で本体リポジトリの行に集約される
- 出力 1 列目は config の `projectAliases` に一致する場合は canonical 名のみ。一致しない場合は `repo_path` そのもの
- alias により同じ canonical 名になる行は cmd 層で session count を合算する
- `--short` は alias 非一致時に `filepath.Base(repo_path)`
- ソート: 直近セッション開始順（降順）
- `--format tsv|json`（デフォルト `tsv`）。JSON のフィールドは `project`, `sessionCount`

### 内容表示（show）

- セッション内容を Markdown で出力
- `show <session-id>` は Claude Code / Codex を横断検索する。同じ `session_id` が複数 source に存在する場合は曖昧エラーとして候補を表示する
- Started 行に `started_at ~ ended_at` の時刻範囲を表示。ended_at がない場合は `started_at ~`
- `--since`/`--until` で期間指定して一括表示
- `--summary N` で各セッションの user メッセージ先頭 N 件を表示（`/clear` と `<local-command-caveat>` はスキップ）。`0` または未指定で従来の全文表示
- `--include-clear` で `/clear`・caveat のスキップを無効化（`--summary >= 1` が前提）
- `--turn N` / `--turn N..M` で指定ターンだけ表示（両端含む）。1 ターンは user メッセージとそれに続く非 user メッセージ（assistant 応答等）。ターン番号は outline と同一の採番（GetMessages の全メッセージ列に対する採番）を共有する。範囲がセッションのターン数を超える場合は本文なしでセッションヘッダのみ出力し exit 0（エラーにしない）。`--turn ""`（空文字）は不正値としてエラー
- `--tail N` で末尾 N ターンだけ表示
- `--turn` と `--tail` は互いに排他。どちらも `--summary` とは併用不可
- `--turn` / `--tail` は `--since`/`--until` の一括表示モードでも各セッションに適用される
- メタデータ `Project` 行は config の `projectAliases` に一致する場合は canonical 名のみ。一致しない場合は `repo_path` をそのまま表示
- `--short` は alias 非一致時に `filepath.Base(repo_path)`
- `--project` は sessions と同じフィルタ規則（`repo_path` への substring マッチ、alias 展開含む）
- `--format markdown|json`（デフォルト `markdown`）。JSON はセッションの配列で、各要素は `source`, `sessionId`, `project`, `title`, `startedAt`, `endedAt`, `messages`（`role`, `content`, `timestamp` の配列）。単一セッション指定でも要素 1 の配列で出す（消費側のパースを一本化するため）。`--summary` / `--turn` / `--tail` のフィルタは `messages` にそのまま反映される

### アウトライン表示（outline）

- `outline <session-id>` で、セッションの user メッセージだけを「ターン番号・時刻・先頭 1 行」の TSV で時系列表示する。長いセッションを全文 show する前に構造を掴む用途
- ターン番号は 1 始まり。sidechain を除いたメッセージ列を時系列に走査し、user メッセージごとに 1 増える（sidechain 除外は show と同じで、採番にも含めない）。最初の user メッセージより前のメッセージはターン 1 に畳み込む
- `/clear` エコーや `<local-command-caveat>` などの合成 user メッセージも 1 ターンとして数え、そのまま表示する（`show --summary` のスキップとは異なる扱い。採番をメッセージ列と 1:1 に保つことを優先する）
- メッセージの時系列順は `timestamp` 昇順、同値は挿入順（rowid）で決定的に並べる（旧形式 Codex rollout は全レコードが同一 timestamp になるため、タイブレーカーがないと採番が実行ごとに揺れる）
- セッション ID の解決は show と同じ（複数 source に一致する場合は曖昧エラーで候補を表示）
- 時刻はローカルタイム `2006-01-02 15:04` 形式
- 先頭 1 行は、前後の空白を除去した本文の最初の行。タブ・改行は空白に置換（TSV 保全）。切り詰めは行わない
- `--format tsv|json`（デフォルト `tsv`）。JSON のフィールドは `turn`, `timestamp`, `firstLine`（`firstLine` は TSV と同じ先頭 1 行抽出だが、タブ・改行の空白置換は行わない）

### 検索（search）

- `search <query> [--since] [--until] [--project]` で全メッセージ本文を横断検索する
- 実装は LIKE 全走査。FTS5 は日本語だと trigram 必須で索引が本文の 2〜3 倍に膨らみ、3 文字未満のクエリが索引で引けないため、LIKE で困るスケールになるまで見送り（本文 42 MB の DB で実測 0.1 秒前後）
- マッチは SQLite LIKE 準拠: 大文字小文字の無視は ASCII のみ、`%`/`_` はワイルドカードとして素通し（Known limitations 参照）
- sidechain メッセージは除外（show と同じ扱い）
- 出力 TSV の列: `session_id`, `time`, `project`, `snippet`。新しい順（メッセージ `timestamp` 降順、同値は rowid 降順）
- `time` はローカルタイム `2006-01-02 15:04` 形式
- `project` は config の `projectAliases` に一致する場合は canonical 名のみ。一致しない場合は `repo_path` をそのまま
- snippet はマッチの前後各 40 文字（rune 単位）。前後が切れている場合は `...` を付加。前後の空白は trim し、タブ・改行は空白に置換（TSV 保全）
- `--since`/`--until` は**メッセージの timestamp 基準**。sessions / show のセッション開始基準とは異なる（検索対象がメッセージのため。`docs/decisions/0013-search-time-filter-on-message-timestamp.md` 参照）
- `--project` は sessions と同じフィルタ規則（`repo_path` への substring マッチ、alias 展開含む）

### JSON 出力（--format json）

機械消費（スクリプト・skill からの利用）向けの構造化出力。判断の経緯は `docs/decisions/0012-json-output-schema.md` 参照。

- 対象コマンド: `sessions` / `projects` / `outline`（`--format tsv|json`、デフォルト `tsv`）、`show`（`--format markdown|json`、デフォルト `markdown`）
- 常に JSON 配列を出力する。結果 0 件は `[]`（show の単一セッション指定も要素 1 の配列）
- フィールド名は camelCase
- タイムスタンプは DB 保存値（RFC3339 UTC）をそのまま出す。ローカルタイム整形は TSV / Markdown 側だけの表示都合とする（タイムゾーン情報を失わないため）
- 文字列は生値（TSV のタブ・改行置換はしない。エスケープは JSON 側で担保される）
- `title` は `custom_title` の生値（Markdown 表示のような session_id フォールバックはしない）
- `project` は alias canonical 表示と `--short` を反映した表示名（alias 一致時は canonical 名のみ、alias 非一致時のデフォルトは `repo_path` の生値）
- 不正な `--format` 値はエラー（`unknown format: ...`）。DB を開く前に検証する
- インデント 2 スペース、HTML エスケープ（`<` `>` `&` の `\uXXXX` 化）は無効

### 設定ファイル（config）

リポジトリのリネーム等で `repo_path` が変わった過去セッションを同じプロジェクトとして扱うための設定。判断の経緯は `docs/decisions/0014-project-alias-config.md` 参照。

- デフォルト配置: `~/.somniloq/config.json`（グローバルフラグ `--config` で変更可能）
- ファイルが存在しない場合は空設定として扱う（エラーにしない）。JSON として壊れている場合はエラー（typo で alias が黙って無効化されるのを防ぐ）
- 形式:

```json
{
  "projectAliases": {
    "somniloq": ["Brimday"]
  }
}
```

- `projectAliases` は「現行名（canonical） → 旧名の配列」のマップ
- `--project` の値がグループの canonical 名または旧名のいずれかに**完全一致**したとき、グループ内の全名称に展開し、いずれかに substring マッチするセッションを対象にする（OR 条件）。完全一致以外は従来どおり値をそのまま 1 パターンとして使う
- 展開は双方向: 旧名を指定しても新名を指定しても同じグループに解決される
- `--project` フィルタ展開の対象は `--project` を持つコマンド（`sessions` / `show` / `search`）
- 表示正規化の対象は project 名を出すコマンド（`sessions` / `show` / `projects` / `search`）。alias グループに一致する `repo_path` / basename は canonical 名だけで表示し、旧名や元のパスを追加フィールドとして出さない
- `projects` 一覧では、alias により同じ canonical 名になる行を cmd 層で合算する。DB の `repo_path` は書き換えない

## CLI インターフェース

```bash
somniloq import                          # Claude Code / Codex の JSONL を差分取り込み
somniloq import --source claude-code     # Claude Code の JSONL だけを差分取り込み
somniloq import --source codex           # Codex の rollout JSONL だけを差分取り込み
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
somniloq show --turn 40..60 <session-id>             # ターン 40〜60 だけ表示
somniloq show --tail 3 <session-id>                  # 末尾 3 ターンだけ表示
somniloq outline <session-id>            # user メッセージをターン番号・時刻・先頭1行で一覧
somniloq sessions --format json          # セッション一覧を JSON で出力
somniloq show --format json <session-id> # セッション内容を JSON で出力（outline / projects も --format json 対応）
somniloq search "auth bug"               # 全メッセージ本文を横断検索
somniloq search --since 7d --project myapp "auth"  # 期間・プロジェクトで絞り込み
somniloq projects                        # プロジェクト一覧
somniloq projects --short                # プロジェクト名を短縮表示
somniloq projects --since 7d             # 直近7日間にセッションがあるプロジェクト
somniloq --db /path/to/somniloq.db ...      # DB パスの指定
somniloq --config /path/to/config.json ...  # 設定ファイルの指定
somniloq --version                          # バージョン表示
```

## SQLite

- デフォルト配置: `~/.somniloq/somniloq.db`（`--db` フラグで変更可能）
- セッション横断で使うため、特定プロジェクトの中には置かない

### テーブル設計

主キー設計と Codex 対応 migration の詳細は `docs/decisions/0004-codex-schema-and-migration.md` 参照。

```sql
-- セッション単位のメタデータ
CREATE TABLE sessions (
    source TEXT NOT NULL,         -- 'claude_code' or 'codex'
    session_id TEXT NOT NULL,     -- Claude Code は UUID、Codex は session_meta.payload.id
    cwd TEXT,                     -- 作業ディレクトリ。会話レコードでは通常非空
    repo_path TEXT,               -- ResolveRepoPath（internal/core/repo_path.go）で解決したリポジトリパス。会話セッションでは通常非空
    git_branch TEXT,
    custom_title TEXT,            -- custom-title レコードから（Claude Code のみ）
    agent_name TEXT,              -- agent-name レコードから（Claude Code のみ）
    version TEXT,                 -- ツールのバージョン
    started_at TEXT,              -- 最初のレコードの timestamp
    ended_at TEXT,                -- 最後のレコードの timestamp
    imported_at TEXT NOT NULL,    -- 取り込み日時
    PRIMARY KEY (source, session_id)
);

-- 会話ターン（user/assistant の text のみ）
CREATE TABLE messages (
    uuid TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    session_id TEXT NOT NULL,
    parent_uuid TEXT,
    role TEXT NOT NULL,           -- 'user' or 'assistant'
    content TEXT NOT NULL,        -- text 部分のみ結合した文字列
    timestamp TEXT NOT NULL,
    is_sidechain BOOLEAN DEFAULT FALSE,
    FOREIGN KEY (source, session_id) REFERENCES sessions(source, session_id)
);

-- 取り込み状態の追跡。主キーは jsonl_path 単独。Claude Code と Codex は
-- ベースディレクトリ（~/.claude/projects/ と ~/.codex/sessions/）が分離して
-- いるため絶対パスだけで一意に特定でき、source は補助情報として保持する。
CREATE TABLE import_state (
    jsonl_path TEXT PRIMARY KEY,  -- JSONL ファイルの絶対パス
    source TEXT NOT NULL,         -- 'claude_code' or 'codex'
    file_size INTEGER,            -- 最終取り込み時のファイルサイズ
    last_offset INTEGER,          -- 最終取り込み行のバイトオフセット
    imported_at TEXT NOT NULL
);
```

## Known limitations

- Claude Code が将来 `cwd` 空の `user`/`assistant` レコードを生成する仕様になった場合、somniloq 側ではそのまま `repo_path` 空で保存する。`projects` 集約で複数リポジトリが空グループに潰れる（`GROUP BY repo_path` 一本のため）。その時点で対応方針を再検討する
- `--project` の値と `search` のクエリは SQLite LIKE のメタ文字（`%`、`_`）を素通しでクエリに渡す（既存挙動の継承）。例: `--project my_repo` は `_` が 1 文字ワイルドカードとして解釈されるため `myXrepo` のような値にも誤マッチする可能性がある

## スキーマ変更への対応方針

- Go の struct タグで既知フィールドのみデコードし、未知は無視（デフォルト挙動）
- `version` フィールドを保存しておけば、問題発生時にバージョン別の切り分けが可能
