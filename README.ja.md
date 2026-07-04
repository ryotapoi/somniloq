# somniloq

Claude Code / Codex のセッションログ（JSONL）を SQLite に取り込み、検索・閲覧する CLI ツール。
`~/.claude/projects/` と `~/.codex/sessions/` 配下の JSONL を解析し、セッション横断で過去の会話を探せるようにする。

## 特徴

- **差分取り込み** — Claude Code / Codex の JSONL を自動検出し、前回からの差分だけを高速に取り込み
- **セッション横断検索** — メッセージ本文を検索し、プロジェクト名・期間で絞り込める
- **長いセッションの部分読み** — サイズを見て、outline で構造を掴み、必要なターンだけ読む
- **Markdown / JSON 出力** — 人が読む Markdown とスクリプト向け JSON を選べる
- **Coding Agent 向け** — スキルから呼び出して、過去のセッションをコンテキストとして活用
- **ローカル完結** — 外部サービス不要。Pure Go + SQLite

## インストール

```bash
go install github.com/ryotapoi/somniloq/cmd/somniloq@latest
```

## クイックスタート

```bash
# Claude Code / Codex のセッションログを取り込む
somniloq import

# セッション一覧を表示
somniloq sessions

# 直近24時間のセッション
somniloq sessions --since 24h

# メッセージ本文を検索
somniloq search --since 7d "auth バグ"

# 長いセッションの構造を見て、必要なターンだけ読む
somniloq outline <session-id>
somniloq show --turn 40..60 <session-id>

# セッションの内容を表示
somniloq show <session-id>

# 直近1週間の全セッションを Markdown で出力
somniloq show --since 7d
```

## コマンド

| コマンド | 説明 |
|---------|------|
| `import` | Claude Code / Codex の JSONL ファイルを SQLite に取り込む |
| `backfill` | 既存 DB の migration / 補正 |
| `sessions` | セッション一覧を表示 |
| `projects` | プロジェクト一覧を表示（セッション数付き） |
| `show` | セッション内容を Markdown 形式で出力 |
| `outline` | セッションの user メッセージをターン番号・時刻・本文サイズ・先頭 1 行で一覧表示 |
| `search` | 全メッセージ本文を横断検索 |

### import

```bash
somniloq import              # 差分取り込み
somniloq import --source claude-code
somniloq import --source codex
somniloq import --full       # 全件再取り込み（確認プロンプトあり）
somniloq import --full --yes # 確認なしで全件再取り込み
```

Claude Code の JSONL を `~/.claude/projects/` から、Codex の rollout JSONL を `~/.codex/sessions/` から取り込む。対象を絞る場合は `--source all|claude-code|codex` を使う。デフォルトは `all`。

`--full` は再取り込み前に somniloq DB 全体を削除する。`somniloq import --source codex --full` を実行した場合も Claude Code の行は削除され、その後 Codex のログだけを取り込む。

エラーは非致命として扱う。parse できない行（壊れた JSON、不正な payload）はスキップしてサマリの `unparsed lines` に計上し、読めないディレクトリ・ファイルもスキップして残りを取り込む。スキップしたエラーは stderr に列挙され、1 件以上あれば exit code は 1 になる。source のディレクトリ自体が存在しない場合は未使用の source として扱い、エラーにしない。

### backfill

```bash
somniloq backfill            # 既存 DB の補正（DELETE 対象があれば確認プロンプト）
somniloq backfill --yes      # 確認なしで補正
```

旧バージョン由来の DB 行を migration / 補正する:

- v0.3 DB を v0.4 スキーマへ移行する（`source` カラム追加、`sessions` の `(source, session_id)` 複合主キー化）
- `repo_path` が `NULL` で `cwd` が非空のセッションを `ResolveRepoPath` で埋める
- `messages` を 1 件も持たない `sessions` 行を削除（v0.2.x のメタ前置 INSERT 残骸を掃除）

v0.4 へアップグレードしたら、取り込み前に `backfill` を 1 回実行する。DELETE 対象が 1 件以上あるときのみ確認プロンプトを出す（デフォルト No）。`--yes` でスキップ可。非対話環境（パイプ・CI 等）では DELETE 対象 1 件以上のとき `--yes` が必須。再実行しても問題ない。

### sessions

```bash
somniloq sessions                        # 全セッション
somniloq sessions --since 24h            # 直近24時間
somniloq sessions --since 7d             # 直近7日間
somniloq sessions --since 2026-03-28     # 特定日以降（ローカルタイム）
somniloq sessions --until 2026-03-28     # 特定日まで（ローカルタイム）
somniloq sessions --since 2026-03-28 --day-boundary 04:00  # 論理日の境界以降
somniloq sessions --since 7d --until 2h  # 7日前から2時間前まで
somniloq sessions --project myapp        # repo_path への substring マッチ
somniloq sessions --short                # alias 非一致時に repo_path の basename
somniloq sessions --format json          # TSV の代わりに JSON 配列
```

出力は TSV 形式: `session_id`, `started_at ~ ended_at`, `logical_day`, `project`, `custom_title`, `message_count`, `body_size`, `non_command_user_turn_count`, `first_non_command_user_line`

`logical_day` はクエリ時に `ended_at`（無ければ `started_at`）から計算する。ローカルタイムの `dayBoundary` を基準にした日付で、セッションを途中で分割しない。

`projectAliases` に一致する repo path / basename は canonical 名のみで表示する。

`body_size` は本文（sidechain 除く）の合計バイト数。`show` する前に大きいセッションかどうかを判定できる。

`non_command_user_turn_count` と `first_non_command_user_line` は読む側のスキップ判定用のヒント。`outline` と同じ sidechain 除外済みの user turn 母集団を使い、trim 後の本文が `/` で始まる user turn と、設定の `commandPatterns` 正規表現に一致する user turn を除外する。CLI 自体はこの値でセッションを捨てない。

`--format json` は `source`, `sessionId`, `project`, `title`, `startedAt`, `endedAt`, `logicalDay`, `messageCount`, `bodySize`, `nonCommandUserTurnCount`, `firstNonCommandUserLine` を持つ JSON 配列を出力する。JSON のタイムスタンプは保存値（RFC3339 UTC）のまま（後述「JSON 出力」参照）。

### projects

```bash
somniloq projects             # 全プロジェクト
somniloq projects --since 7d  # 直近7日間にセッションがあるプロジェクト
somniloq projects --short     # alias 非一致時に repo_path の basename
somniloq projects --format json
```

出力は TSV 形式: `project`, `session_count`。`--format json` では `project`, `sessionCount`。alias グループは canonical 名で表示・集計する。

### show

```bash
somniloq show <session-id>                              # 特定セッション
somniloq show --since 24h                               # 直近24時間の全セッション
somniloq show --since 2026-03-28 --until 2026-03-29     # 特定期間
somniloq show --since 7d --project myapp                # プロジェクト絞り込み
somniloq show --summary 1 --since 24h                   # 各セッションの冒頭ユーザーメッセージ 1 件
somniloq show --summary 3 --since 24h                   # 各セッションの冒頭ユーザーメッセージ 3 件
somniloq show --short --since 24h                       # alias 非一致時に repo_path の basename
somniloq show --turn 40..60 <session-id>                # ターン 40〜60 だけ表示
somniloq show --tail 3 <session-id>                     # 末尾 3 ターンだけ表示
somniloq show --format json <session-id>                # Markdown の代わりに JSON
```

`--turn` / `--tail` のターン番号は `outline` と同じ採番（user メッセージごとに 1 増える 1 始まり）。`outline` で目星を付けた範囲だけを読む用途。1 ターンには user メッセージとそれに続く応答が含まれる。`--turn` と `--tail` は互いに排他で、`--summary` とも併用できない。一括表示モード（`--since`/`--until`）では各セッションに個別に適用される。

`--format json` はセッションの JSON 配列を出力する（単一セッション指定でも要素 1 の配列）。各要素は `source`, `sessionId`, `project`, `title`, `startedAt`, `endedAt`, `messages`（`role`, `content`, `timestamp` の配列）を持つ。`--summary` / `--turn` / `--tail` のフィルタは `messages` にそのまま反映される。

### outline

```bash
somniloq outline <session-id>                 # user メッセージをターン番号・時刻・本文サイズ・先頭1行で一覧
somniloq outline --format json <session-id>  # TSV の代わりに JSON
```

長いセッションを全文 `show` する前に構造を掴むためのコマンド。出力は TSV 形式: `turn`, `time`, `body_size`, `first_line`。ターン番号は user メッセージごとに 1 増える 1 始まりの連番（sidechain は除外）。`body_size` はその turn に属する非 sidechain メッセージ本文（応答を含む）の UTF-8 バイト合計。`--format json` では `turn`, `timestamp`, `bodySize`, `firstLine`。

### search

```bash
somniloq search "auth バグ"                          # 全メッセージ本文を検索
somniloq search --since 7d "auth"                    # 直近 7 日間に書かれたメッセージ
somniloq search --since 2026-03-28 --day-boundary 04:00 "auth"  # その日の 04:00 以降
somniloq search --since 7d --project myapp "auth"    # プロジェクトで絞り込み
```

出力は TSV 形式: `session_id`, `time`, `project`, `snippet`（最初のマッチ前後の本文）。新しい順。マッチは SQLite LIKE 準拠で、大文字小文字の無視は ASCII のみ、`%`/`_` はワイルドカードとして解釈される。`sessions`/`show` と異なり、`--since`/`--until` は**メッセージ**の timestamp（内容が書かれた時刻）で絞る。date-only のフィルタは `dayBoundary` を使う。sidechain メッセージは除外。

### JSON 出力

`sessions` / `projects` / `outline`（`--format tsv|json`）と `show`（`--format markdown|json`）はスクリプト向けの JSON 出力に対応している。全コマンド共通の規則:

- 常に JSON 配列。結果 0 件は `[]`
- タイムスタンプは保存値（RFC3339 UTC）のまま。ローカルタイム整形は行わない
- 文字列は生値（タブ・改行の置換はしない。エスケープは JSON 側で担保）。`title` は custom title の生値で session-id へのフォールバックはしない
- `project` は設定された alias canonical 表示を反映する。alias 非一致時は `--short` を反映し、付けなければ `repo_path` の生値

## 設定ファイル

`~/.somniloq/config.json` に任意で置ける（グローバルフラグ `--config` で変更可能）。ファイルが無い場合は問題なし。JSON が壊れている場合はエラー。

```json
{
  "projectAliases": {
    "新しい名前": ["古い名前"]
  },
  "commandPatterns": ["^日報生成"],
  "dayBoundary": "04:00"
}
```

`projectAliases` は、時期によって名前が変わった同一プロジェクト（リネームしたリポジトリ等）をグループ化する: 現行名 → 旧名の配列。`--project` の値がグループ内のいずれかの名前に完全一致すると、フィルタがグループ全体に展開され、どちらの名前で記録されたセッションも見つかる。一致しない値は従来どおり。フィルタ展開の対象は `sessions` / `show` / `search`。`sessions` / `show` / `projects` / `search` の project 表示は、保存された `repo_path` または basename が alias グループに一致する場合に canonical 名のみを出し、`projects` は canonical 名で合算する。

`commandPatterns` は `sessions` のスキップ判定用列だけで使う Go 正規表現のリスト。各 pattern は trim 済みの user message 本文全体に対して評価する。不正な正規表現は壊れた JSON と同じく config 読み込みエラーになり、typo を黙って無効化しない。

`dayBoundary` は論理日の開始時刻をローカルタイムの `HH:MM` で指定する。未指定時は `00:00`。`sessions` / `search` の `--day-boundary` でコマンドごとに上書きできる。date-only の `--since` / `--until` と `sessions` の `logical_day` 列だけに効き、保存済み timestamp は生のままなので、境界を変えても再 import は不要。

## 共通オプション

| オプション | 説明 | デフォルト |
|-----------|------|-----------|
| `--db <path>` | SQLite データベースのパス | `~/.somniloq/somniloq.db` |
| `--config <path>` | 設定ファイル（JSON）のパス | `~/.somniloq/config.json` |
| `--version` | バージョンを表示して終了 | — |

> SQLite 3.35 以上が必要（`ALTER TABLE ... DROP COLUMN` を使用）。同梱の `modernc.org/sqlite` ドライバが新しめの SQLite を含むので、別途のインストールは不要。

### 時刻フィルタ

`--since` と `--until` は以下の形式に対応:

| 形式 | 例 | 意味 |
|-----|-----|------|
| 相対時刻 | `30m`, `24h`, `7d` | 現在からの相対時間 |
| 絶対日付 | `2026-03-28` | `sessions` / `search` では設定されたローカル `dayBoundary`、それ以外はローカルタイムの 00:00 |
| 絶対日時 | `2026-03-28T15:00` | ローカルタイムの指定時刻 |

## v0.4 へのアップグレード

v0.4 では Codex 対応に伴い、セッションキーに `source` を含める。既存 DB に対して一度 migration / 補正を入れる必要がある。

1. **DB をバックアップ。** `backfill` は孤立行を削除するため、`~/.somniloq/somniloq.db` を別名でコピーしておく。
2. **v0.4 バイナリを入れて以下を実行:**
   ```bash
   somniloq backfill
   ```
   v0.3 の行を v0.4 スキーマへ移行し、旧来の行の `repo_path` を埋め、`messages` を 1 件も持たない `sessions` 行（v0.2.x のメタ前置 INSERT 残骸）を削除する。
3. **現在のログを取り込む。**
   ```bash
   somniloq import
   ```
4. **任意 — 退避済みの JSONL を補充する。** `~/.claude/projects/` から JSONL を別所に移していた場合は、現状に無い分だけコピーして再取り込みする:
   ```bash
   cp -rn /path/to/old-projects/. ~/.claude/projects/
   somniloq import --full --yes
   ```

### CLI 挙動の変更点

- `--project` は `repo_path` への substring マッチ一本になった。旧来の `project_dir` フォールバックは廃止。`repo_path` が `NULL` のままの古い行は `somniloq backfill` を実行するまで `--project` にヒットしない。
- `sessions` / `projects` の TSV 出力は `repo_path` 由来の `project` を出す（`project_dir` フォールバック表記なし）。設定された project alias は canonical 名で表示する。
- `--short` は alias 非一致時に `filepath.Base(repo_path)` を出す。
- `import` はデフォルトで Claude Code / Codex の両方を取り込む。片方だけ取り込む場合は `--source claude-code` または `--source codex` を使う。

## ドキュメント

- [プロダクト目的・非目標](docs/rules/mission.md)
- [主要機能・CLI・テーブル設計](docs/rules/scope.md)
- [モジュール構成と依存方向](docs/rules/architecture.md)

## ライセンス

[MIT License](LICENSE)
