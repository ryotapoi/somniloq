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
somniloq search --config default "auth bug"        # 本文の語からまとまりを探す
somniloq show --config default <REF> --messages 12:18 # 発言番号で読む
```

全 DB コマンドで `--config NAME_OR_PATH` が必須。コマンド名の前でも後でも指定できる。search のフラグは検索語より前に置く。`sessions` と `search` の完全 `slq1:...` REF は、別入力にある同名セッション ID も区別する。そのまま `show` に渡す。裸 ID・短縮 REF は受理しない。

## コマンド

| コマンド | 用途 |
|----------|------|
| `config init` | DB を開かず TOML 設定を作成する。 |
| `import` | 新しいログを取り込む。`--input PATH` の繰り返しで root を選び、`--source claude-code`、`codex`、`cursor-agent` で source を絞る。 |
| `migrate` | 固定した既知の旧 DB snapshot を別 DB に移し、残存 Codex ログで証明できた旧行だけ置換する。 |
| `sessions` | セッション一覧。`--since 24h` はセッション時刻、`--imported-since 24h` は最近保存・更新されたセッションで絞る。 |
| `projects` | プロジェクトとセッション件数を一覧する。 |
| `search` | 語・入力・source・project から作業のまとまりを選ぶ。既知 REF は全一致詳細で探す。 |
| `show` | 複数会話の原文を TSV / JSON で取得し、発言フィルタ・ページ・一行表示を選ぶ。 |

`import` は差分取り込み。複数 input 条件は OR、source 条件とは交差する。**`--full` は選択入力の会話・差分状態だけを再構築し、他入力を保持する。** 対象入力の元ログが残っていることを確認して使う。確認プロンプトは `--yes` で省略でき、非対話環境では `--yes` が必須。

Codex の取り込みは rollout の本人会話と継承文脈を区別する。最初の有効な session metadata で本人を固定し、明示された直接親 ID は親が後着しても保持する。text block、元 path・物理行、元の発言日時（未知を含む）、1 始まりの発言番号を保存する。同じ本人の複数 rollout は相対 path・物理行順に並べ、前方の内容が変われば差分取り込みでも順序を再構築する。子本人の完全 REF を指定すればその会話を読める。show は元の発言番号と原文 blocks を返します。

新 DB は schema revision 1。通常コマンドは旧形式・非対応 DB（以前の root-only revision 1 shape を含む）を変更せず拒否する。既知の旧形式は `somniloq migrate --config archive --from ./archive-snapshot.db` で移せる。設定の `db` を未存在または空の移行先にし、残存 Codex ログの全 root を設定する。元は sidecar のない固定 standalone snapshot にする。同じ snapshot と完了 receipt がある移行先には再実行できる。所属不明・ログ欠落・他 source の旧履歴は保持し、legacy REF で参照できる。詳細は [移行契約](docs/specs/v0.14.0-migration.md) を参照。read コマンドは未存在 DB を作成せず拒否する。

フラグ・出力形式は `somniloq <command> --help` を参照。`sessions`、`projects`、`search`、`show` は `--format tsv|json` に対応する。show と search の JSON は envelope、sessions/projects は配列。session なし search は本文抜粋を含まないまとまり一覧。Claude Code の子・孫は root と sessionId を共有しても独立した本人 REF で取り込み・検索・閲覧でき、本人 sidechain の原文も保持する。直接親は同一物理ファイルの Agent/Task call と構造化結果を厳密に照合し、path の root 所属とは区別する。走査・読み取りが不完全な入力は前回保存の本文・関係・cursor を保持し、他入力の取り込みを続ける。`search --session REF PATTERN` は共通 resolver で指定本人と確定子孫だけを検索し、祖先・兄弟・root 所属だけの子を含めない。Go regexp の全一致箇所を返す。複数 -e、全 pattern を固定文字列にする -F、本文集合全体の --all AND を指定できる。

複数の完全 REF を一回の呼び出しで渡し、指定日の実発言だけを Daily Note の材料として取得できます。REF の指定順・各会話の元の発言番号順を保ち、`--descendants` は確定子孫だけを展開して重複会話を除きます。role・発言番号・日時で絞った後に、発言単位で limit / offset / tail を適用します。`--one-line` は text だけを最初の一行へ短縮し、blocks は原文を保ちます。既定 TSV、JSON は `{items,total,count,limit,offset,hasMore,nextOffset}` envelope です。show の日時は日付または zone 付き RFC3339 で指定し、相対時刻は受理しません。旧 outline・summary・turn・表示除外・Markdown・REF なし期間入口は廃止しました。一覧は REF と metadata を返し、本文抜粋・turn を含みません。詳細 search の messageNumber は show と同じ原文番号です。詳細は [現行仕様](docs/rules/scope.md#内容表示show) を参照してください。

```sh
# REF1 / REF2 は sessions / search からコピーした完全 REF
somniloq show --config default REF1 REF2 --since 2026-10-01 --until 2026-10-01 --format json
somniloq show --config default REF --role user --one-line
somniloq show --config default REF --messages 12:18 --limit 50 --format json
```

一覧の JSON は `{items,total,count,limit,offset,hasMore,nextOffset}`、既定20件です。pattern 省略で一覧、複数 -e は OR、--all は候補本文集合で AND、-F は固定文字列。input/source は繰り返しの OR、条件種間は AND。project は末尾名の大小文字区別 substring と完全一致 alias 展開です。members は全まとまり、matchedMembers は候補本人。root metadata を子で補完せず、全 members の本人原文日時で last 降順・未知最後・group key 順に整列します。limit=0 と末尾超過も total を返します。

次は既定ページから REF を選ぶ例です。since/until/dayBoundary は暫定的に従来の発言時刻 filter を使い、pattern なしでも期間内の本文が必要です。全件が必要なら hasMore/nextOffset を見てページを取得します。新しい time-mode は後続実装です。

```sh
set -- $(somniloq search --config default --since 2026-10-01 --until 2026-10-01 --format json | jq -r '.items[].members[]' | sort -u)
if [ "$#" -gt 0 ]; then
  somniloq show --config default "$@" --since 2026-10-01 --until 2026-10-01 --format json
fi
```

完全 REF は空白や glob 文字を含まず、この sort 順が show の会話順になります。

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

任意の `projectAliases` は改名したプロジェクトをまとめる（`[projectAliases]` の下に `new-name = ["old-name"]`）。グループの重複は拒否する。任意の `dayBoundary` はローカル時刻で論理日の開始を指定する。未知キー・不正値はエラー。旧 JSON は探索・変換せず、`excludeUserMessagePatterns` は設定キーとして受理しない。show の表示除外フラグも提供しない。

`--config default` は設定名、`--config ./archive.toml` は path 指定。設定の未指定・欠落は exit 2 と作成案内を返し、DB・設定を自動生成しない。

## 詳細

- [CLI の振る舞いと設定](docs/rules/scope.md)
- [目的と非目標](docs/rules/mission.md)
- [変更履歴](CHANGELOG.ja.md)

## ライセンス

[MIT License](LICENSE)

既知 REF の詳細検索は既定全件で、明示 limit/offset は一致箇所単位です。原文 UTF-8 byte 位置、matchText、行全体の lineText と番号を返し、同じ REF/番号で show に戻れます。フラグは位置 PATTERN より前に置き、patternIndexes は位置 PATTERN が先頭、次に -e の指定順です。一覧でも同じ -e/-F/--all を使えます。

```sh
somniloq search --config default --session REF -e "Inherited question" -e "Child answer" --all --format json
somniloq search --config default --session REF -F --limit 20 --offset 20 "auth bug"
somniloq show --config default REF --messages 1:1 --format json
```
