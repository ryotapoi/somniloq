# somniloq

Claude Code のセッションログ（JSONL）を SQLite に取り込み、検索・閲覧する CLI ツール。
`~/.claude/projects/` 配下の JSONL を解析し、セッション横断で過去の会話を探せるようにする。

## 特徴

- **差分取り込み** — `~/.claude/projects/` 配下の JSONL を自動検出し、前回からの差分だけを高速に取り込み
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
# セッションログを取り込む
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
| `import` | JSONL ファイルを SQLite に取り込む |
| `sessions` | セッション一覧を表示 |
| `projects` | プロジェクト一覧を表示（セッション数付き） |
| `show` | セッション内容を Markdown 形式で出力 |

### import

```bash
somniloq import              # 差分取り込み
somniloq import --full       # 全件再取り込み（確認プロンプトあり）
somniloq import --full --yes # 確認なしで全件再取り込み
```

### sessions

```bash
somniloq sessions                        # 全セッション
somniloq sessions --since 24h            # 直近24時間
somniloq sessions --since 7d             # 直近7日間
somniloq sessions --since 2026-03-28     # 特定日以降（ローカルタイム）
somniloq sessions --until 2026-03-28     # 特定日まで（ローカルタイム）
somniloq sessions --since 7d --until 2h  # 7日前から2時間前まで
somniloq sessions --project myapp        # プロジェクト名で絞り込み
somniloq sessions --short                # プロジェクト名を短縮表示
```

出力は TSV 形式: `session_id`, `started_at ~ ended_at`, `project_dir`, `custom_title`, `message_count`

### projects

```bash
somniloq projects             # 全プロジェクト
somniloq projects --since 7d  # 直近7日間にセッションがあるプロジェクト
somniloq projects --short     # プロジェクト名を短縮表示
```

出力は TSV 形式: `project_dir`, `session_count`

### show

```bash
somniloq show <session-id>                              # 特定セッション
somniloq show --since 24h                               # 直近24時間の全セッション
somniloq show --since 2026-03-28 --until 2026-03-29     # 特定期間
somniloq show --since 7d --project myapp                # プロジェクト絞り込み
somniloq show --summary --since 24h                     # 各セッションの冒頭のみ
somniloq show --short --since 24h                       # プロジェクト名を短縮表示
```

## 共通オプション

| オプション | 説明 | デフォルト |
|-----------|------|-----------|
| `--db <path>` | SQLite データベースのパス | `~/.somniloq/somniloq.db` |

### 時刻フィルタ

`--since` と `--until` は以下の形式に対応:

| 形式 | 例 | 意味 |
|-----|-----|------|
| 相対時刻 | `30m`, `24h`, `7d` | 現在からの相対時間 |
| 絶対日付 | `2026-03-28` | ローカルタイムの 00:00 |
| 絶対日時 | `2026-03-28T15:00` | ローカルタイムの指定時刻 |

## ドキュメント

- [プロダクト目的・非目標](rules/mission.md)
- [主要機能・CLI・テーブル設計](rules/scope.md)
- [モジュール構成と依存方向](rules/architecture.md)

## ライセンス

[MIT License](LICENSE)
