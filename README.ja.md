# somniloq

somniloq は Claude Code / Codex / Cursor Agent の JSONL セッションログを SQLite に取り込み、セッションを横断して会話を検索・閲覧するローカル CLI。TOML 設定で DB と source ごとの複数ログ root を指定する。

[English README](README.md)

## インストール

Go 1.27.1 以降が必要。

```bash
go install github.com/ryotapoi/somniloq/cmd/somniloq@latest
```

## クイックスタート

```bash
somniloq config init                              # default TOML を生成（DB は未作成）
somniloq import --config default                  # 全設定入力を取り込む
somniloq sessions --config default --since 7d      # 最近のセッションを探す
somniloq search --config default "auth bug"        # 本文を検索する
somniloq outline --config default <REF>            # sessions/search の完全 REF を使う
somniloq show --config default --turn 12..18 <REF> # 必要なターンを読む
```

全 DB コマンドで `--config NAME_OR_PATH` が必須。コマンド名の前でも後でも指定できる。search のフラグは検索語より前に置く。`sessions` と `search` の完全 `slq1:...` REF は、別入力にある同名セッション ID も区別する。そのまま `show` / `outline` に渡す。裸 ID・短縮 REF は受理しない。

## コマンド

| コマンド | 用途 |
|----------|------|
| `config init` | DB を開かず TOML 設定を作成する。 |
| `import` | 新しいログを取り込む。`--input PATH` の繰り返しで root を選び、`--source claude-code`、`codex`、`cursor-agent` で source を絞る。 |
| `migrate` | 固定した既知の旧 DB snapshot を別 DB に移し、残存 Codex ログで証明できた旧行だけ置換する。 |
| `sessions` | セッション一覧。`--since 24h` はセッション時刻、`--imported-since 24h` は最近保存・更新されたセッションで絞る。 |
| `projects` | プロジェクトとセッション件数を一覧する。 |
| `search` | メッセージ本文を検索する。`--project` や `--since` で絞れる。 |
| `outline` | 完全 REF で選んだ会話の user ターンを一覧する。 |
| `show` | 会話を Markdown で読む。`--turn` や `--tail` で一部だけ読める。 |

`import` は差分取り込み。複数 input 条件は OR、source 条件とは交差する。**`--full` は選択入力の会話・差分状態だけを再構築し、他入力を保持する。** 対象入力の元ログが残っていることを確認して使う。確認プロンプトは `--yes` で省略でき、非対話環境では `--yes` が必須。

Codex の取り込みは rollout の本人会話と継承文脈を区別する。最初の有効な session metadata で本人を固定し、明示された直接親 ID は親が後着しても保持する。text block、元 path・物理行、元の発言日時（未知を含む）、1 始まりの発言番号を保存する。同じ本人の複数 rollout は相対 path・物理行順に並べ、前方の内容が変われば差分取り込みでも順序を再構築する。子本人の完全 REF を指定すればその会話を読める。現行 `show`、`outline` の出力形式は従来のまま。

新 DB は schema revision 1。通常コマンドは旧形式・非対応 DB（以前の root-only revision 1 shape を含む）を変更せず拒否する。既知の旧形式は `somniloq migrate --config archive --from ./archive-snapshot.db` で移せる。設定の `db` を未存在または空の移行先にし、残存 Codex ログの全 root を設定する。元は sidecar のない固定 standalone snapshot にする。同じ snapshot と完了 receipt がある移行先には再実行できる。所属不明・ログ欠落・他 source の旧履歴は保持し、legacy REF で参照できる。詳細は [移行契約](docs/specs/v0.14.0-migration.md) を参照。read コマンドは未存在 DB を作成せず拒否する。

フラグ・出力形式は `somniloq <command> --help` を参照。`sessions`、`projects`、`search`、`outline` は `--format json` に対応し、`show` は Markdown または JSON で出力する。JSON は引き続き配列、search は literal substring 検索。Claude Code の子・孫は root と sessionId を共有しても独立した本人 REF で取り込み・検索・閲覧でき、本人 sidechain の原文も保持する。直接親は同一物理ファイルの Agent/Task call と構造化結果を厳密に照合し、path の root 所属とは区別する。走査・読み取りが不完全な入力は前回保存の本文・関係・cursor を保持し、他入力の取り込みを続ける。`search --session REF QUERY` は共通 resolver で指定本人と確定子孫だけを検索し、祖先・兄弟・root 所属だけの子を含めない。query 必須の LIKE 検索を維持する。まとまり検索・新しい原文取得の show は後続実装。

## 設定

`somniloq config init [NAME] [--output PATH] [--db PATH]` の既定名は `default`、出力は `~/.somniloq/config/NAME.toml`、DB は `~/.somniloq/NAME.db`。parent directory を作成し、symlink を含む既存宛先への上書きを拒否する。stdout は設定の絶対 path 一行。`--db` は init だけで利用できる。

生成する設定には次の3入力がある。

```toml
db = "~/.somniloq/default.db"
dayBoundary = "00:00"

[[inputs]]
name = "Claude"
source = "claude-code"
root = "~/.claude/projects"

[[inputs]]
name = "Codex"
source = "codex"
root = "~/.codex/sessions"

[[inputs]]
name = "Cursor"
source = "cursor-agent"
root = "~/.cursor/projects"
```

追加 root は `[[inputs]]` を増やす。同じ source と実体 root は一回だけ走査する。name は任意の表示名で identity を変えない。未存在 root は0件。相対 db/root は設定の実体親 directory 基準で、設定 symlink も実体解決する。先頭 `~` または `~/` だけを home へ展開し、環境変数は展開しない。

任意の `projectAliases` は改名したプロジェクトをまとめる（`[projectAliases]` の下に `new-name = ["old-name"]`）。グループの重複は拒否する。任意の `dayBoundary` はローカル時刻で論理日の開始を指定する。未知キー・不正値はエラー。旧 JSON は探索・変換せず、`excludeUserMessagePatterns` は設定キーとして受理しない。現行 outline / summary の表示除外は CLI の `--exclude-user-message-pattern` で指定できる。

`--config default` は設定名、`--config ./archive.toml` は path 指定。設定の未指定・欠落は exit 2 と作成案内を返し、DB・設定を自動生成しない。

## 詳細

- [CLI の振る舞いと設定](docs/rules/scope.md)
- [目的と非目標](docs/rules/mission.md)
- [変更履歴](CHANGELOG.ja.md)

## ライセンス

[MIT License](LICENSE)
