# somniloq

Claude Code / Codex のセッションログ（JSONL）を SQLite に取り込み、検索・閲覧する CLI ツール。
`~/.claude/projects/` と `~/.codex/sessions/` 配下の JSONL を解析し、セッション横断で過去の会話を探せるようにする。

## 特徴

- **差分取り込み** — Claude Code / Codex の JSONL を自動検出し、前回からの差分だけを高速に取り込み
- **セッション横断検索** — プロジェクト名・期間で絞り込み、過去の会話をすぐに見つけられる
- **Markdown 出力** — セッション内容を Markdown 形式で出力。Daily Note や振り返りに活用できる
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

### import

```bash
somniloq import              # 差分取り込み
somniloq import --source claude-code
somniloq import --source codex
somniloq import --full       # 全件再取り込み（確認プロンプトあり）
somniloq import --full --yes # 確認なしで全件再取り込み
```

Claude Code の JSONL を `~/.claude/projects/` から、Codex の rollout JSONL を `~/.codex/sessions/` から取り込む。対象を絞る場合は `--source all|claude-code|codex` を使う。デフォルトは `all`。

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
somniloq sessions --since 7d --until 2h  # 7日前から2時間前まで
somniloq sessions --project myapp        # repo_path への substring マッチ
somniloq sessions --short                # repo_path の basename
```

出力は TSV 形式: `session_id`, `started_at ~ ended_at`, `repo_path`, `custom_title`, `message_count`

### projects

```bash
somniloq projects             # 全プロジェクト
somniloq projects --since 7d  # 直近7日間にセッションがあるプロジェクト
somniloq projects --short     # repo_path の basename
```

出力は TSV 形式: `repo_path`, `session_count`

### show

```bash
somniloq show <session-id>                              # 特定セッション
somniloq show --since 24h                               # 直近24時間の全セッション
somniloq show --since 2026-03-28 --until 2026-03-29     # 特定期間
somniloq show --since 7d --project myapp                # プロジェクト絞り込み
somniloq show --summary 1 --since 24h                   # 各セッションの冒頭ユーザーメッセージ 1 件
somniloq show --summary 3 --since 24h                   # 各セッションの冒頭ユーザーメッセージ 3 件
somniloq show --short --since 24h                       # repo_path の basename
```

## 共通オプション

| オプション | 説明 | デフォルト |
|-----------|------|-----------|
| `--db <path>` | SQLite データベースのパス | `~/.somniloq/somniloq.db` |
| `--version` | バージョンを表示して終了 | — |

> SQLite 3.35 以上が必要（`ALTER TABLE ... DROP COLUMN` を使用）。同梱の `modernc.org/sqlite` ドライバが新しめの SQLite を含むので、別途のインストールは不要。

### 時刻フィルタ

`--since` と `--until` は以下の形式に対応:

| 形式 | 例 | 意味 |
|-----|-----|------|
| 相対時刻 | `30m`, `24h`, `7d` | 現在からの相対時間 |
| 絶対日付 | `2026-03-28` | ローカルタイムの 00:00 |
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
- `sessions` / `projects` の TSV 出力は `repo_path` をそのまま出す（`project_dir` フォールバック表記なし）。
- `--short` は常に `filepath.Base(repo_path)`。
- `import` はデフォルトで Claude Code / Codex の両方を取り込む。片方だけ取り込む場合は `--source claude-code` または `--source codex` を使う。

## ドキュメント

- [プロダクト目的・非目標](rules/mission.md)
- [主要機能・CLI・テーブル設計](rules/scope.md)
- [モジュール構成と依存方向](rules/architecture.md)

## ライセンス

[MIT License](LICENSE)
