# Scope

本書が CLI 仕様・コマンド挙動・スキーマの正。README.md / README.ja.md は本書の派生ビューなので、本書のこれらの記述を変更したら README 両方を同期する。

## 主要機能

### 取り込み（import）

source（DB 内部値は `claude_code` / `codex` / `cursor_agent`）ごとに専用の adapter で取り込む。共通の正規化スキーマ（`sessions` / `messages`）に保存する点は共通だが、ファイル配置・レコード形式・差分検出キーは source ごとに異なる。

`somniloq import` はデフォルトで Claude Code、Codex、Cursor Agent を同じ SQLite DB に取り込む。対象を絞る場合は CLI 表記の `--source all|claude-code|codex|cursor-agent` を使う。

#### エラー処理と取り込みサマリ（source 共通）

- 取り込み終了時に `Imported <n> files (<scanned> scanned, <skipped> skipped, <failed> failed, <unparsed> unparsed lines)` を stdout に出力する
- parse / 正規化できない行（壊れた JSON、不正な payload）はスキップして続行し、`unparsed lines` に計上する。source が意図的に無視するレコード型（未知 type・非 message レコード・空行等）はカウントしない（`docs/decisions/0009-unparsed-line-visibility.md`）
- parse / 正規化失敗のうち先頭 5 件を `ファイル:行番号: エラー内容` 形式で stderr に出力する。この診断だけでは exit code を変更しない
- `unparsed lines` は「その実行で読んで解釈できなかった行数」を意味する。`import_state` の offset が進まないファイル（メタセッション等）の unparsed 行は、`import` のたびに繰り返し計上される
- ディレクトリ走査で読めないディレクトリはスキップして続行し、発見できたファイルは取り込む。ファイル単位の取り込み失敗も同様にスキップして続行する（`docs/decisions/0010-non-fatal-scan-errors.md`）
- ディレクトリ走査・ファイル単位の取り込みエラーは stderr に列挙し、1 件以上あれば exit code は 1（取り込み自体は部分的に完了している）
- source のルートディレクトリが存在しない場合は、その source を未使用として扱いエラーにしない

#### Claude Code 用（`somniloq import --source claude-code`）

- `~/.claude/projects/` を走査し、各 JSONL ファイルを列挙
- `import_state` と照合し、未取り込み or サイズ増加分を検出（差分取り込み）
- 各 JSONL を行単位で読み、`type` でフィルタ
- `user`/`assistant` → messages テーブルへ（text 部分のみ抽出）
- `user`/`assistant` レコードが初出のときだけ `sessions` 行を作成する。text 抽出結果が空の会話レコード（`tool_use` のみ・添付のみ・空白のみ）では、text 非空判定の前に session を保存するため `messages` 0 件の session が残る
- メタセッション（`custom-title` / `agent-name` 単独で `user`/`assistant` を持たない）は DB に保存しない。当該ファイルの `import_state` も進めず、後で会話レコードが追記されたときに先頭から再読み込みできる状態を維持する
- `user`/`assistant` の `cwd` から `repo_path` を解決して sessions に保存。`cwd` は会話レコードでは通常非空のため、会話セッションでは `repo_path` も通常非空（`ResolveRepoPath` 手順 4 で `cwd` 自体を返すため、`cwd` 非空なら必ず解決される）
- `custom-title` / `agent-name` レコードは、ファイル走査終了時点で対応する `sessions` 行が存在するときのみ反映する
- `import_state` を更新
- `--full` フラグで全件再取り込み（確認プロンプトあり、デフォルト No）
  - `--yes` で確認をスキップ
  - 非対話環境（パイプ、CI 等）では `--yes` が必須
  - `--source` 指定時も DB 全体を削除し、指定 source だけを再取り込みする

#### repository の解決と既存データ

Claude Code と Codex は共通の `ResolveRepoPath` で `cwd` を解決する。空 cwd は空、`/.claude/worktrees/` を含む cwd は最初の marker より前を優先する。それ以外は Git の top-level と worktree 情報を使い、実在する通常の linked worktree とそのサブディレクトリも本体 repository の root に集約する。通常 repository と submodule はそれぞれ自身の root を使い、Git が解決できない cwd は元の値を保持する。消失した一般 worktree の本体は推測しない。

保存済みの非 NULL `repo_path` は自動補正されず、不変ファイルは差分 import でスキップされる。元ログと対象 worktree が残っていれば `somniloq import --full --yes` で再構築できる。ただし source 制限にかかわらず DB 全体を削除して指定 source だけを再取り込みするため、保持したい全 source の元ログを確認する。

旧形式 DB 向けの専用 upgrade・データ補正手段は提供しない。`OpenDB` による一般的な schema 管理は維持するが、v0.3 形式から現在の source 付き schema への移行成功は保証しない。

#### Codex 用（`somniloq import --source codex`）

- `~/.codex/sessions/` 配下の日付ディレクトリを再帰走査し、rollout JSONL を列挙
- 各 JSONL を行単位で読み、`response_item` かつ `payload.type == "message"` かつ `role in ("user", "assistant")` のレコードのみを取り込み対象とする
- `payload.content` は `input_text` / `output_text` / `text` block の `text` のみを抽出し、複数 block は空行区切りで結合する
- `session_id` は `session_meta.payload.id` を使う
- `session_meta.payload.cwd` から `repo_path` を解決して sessions に保存（解決ロジックは Claude Code 側と共有）
- `git_branch` は `session_meta.payload.git.branch`、`version` は `session_meta.payload.cli_version` から保存する
- `messages.uuid` の一意性は `(rollout_path, line_number)` ベースで判定（Codex のレコードは Claude Code のような UUID を持たないため）
- 差分取り込みで追記分だけを読む場合も、offset 直前までの `session_meta` を先に読み直して session メタデータを復元する
- 差分取り込み・`--full` 等のオプション体系は `import` と揃える

#### Cursor Agent 用（`somniloq import --source cursor-agent`）

- `~/.cursor/projects/` 配下の `<project-slug>/agent-transcripts/<session-id>/<session-id>.jsonl` だけを取り込む
- `user` / `assistant` の `message.content` から `text` block だけを配列順に空行で連結し、tool、turn、未知正常 record、空行は保存しない
- ログにない timestamp、cwd、repository、version、title、usage、parent は補完しない
- `messages.uuid` の一意性は source、path、物理行に基づく。差分取り込み時も空行・無視行・unparsed 行を含む物理行番号を維持する
- 差分取り込み・`--full` 等のオプション体系は `import` と揃える

### 時刻フィルタの共通規則

- 保存済み timestamp の NULL・空文字列・RFC3339 として解釈できない値は、比較上は時点不明として扱う。範囲 filter には一致せず、filter なしでは行・本文を保持し、並び順は既存の NULL 順（昇順で先、降順で後、message 同値は rowid）に従う。不正な非空文字列は保存・JSON 出力・時刻表示にそのまま残し、時刻を補完しない。CLI の不正な時刻引数はエラーとする。
- RFC3339 instant 入力の小数秒はミリ秒へ切り捨てず、ナノ秒精度で保持して時点として比較し、offset 表現が異なっても同じ時点なら等しい。`--since` は包含下限、`--until` は排他上限

### セッション一覧（sessions）

- セッション一覧を表示
- `--since`/`--until` で時刻フィルタ（相対: `24h`, `7d`、ローカル絶対値: `2026-03-28`, `2026-03-28T15:00`、RFC3339 instant: `2026-03-28T15:00:00Z`, `2026-03-29T00:00:00+09:00`）。絶対日付と分精度日時はローカルタイム。RFC3339 instant は `Z` または numeric offset で指定した正確な時点として解釈する。date-only（`YYYY-MM-DD`）は `dayBoundary`（未設定時 `00:00`、`--day-boundary HH:MM` で上書き可）を起点に解釈する。相対時刻と日時は `dayBoundary` の影響を受けない。出力のタイムスタンプもローカルタイム（`2006-01-02 15:04` 形式）
- `--imported-since` は session の `imported_at` を基準にした包含下限。相対時刻、ローカル日付、分精度日時、RFC3339 instant は `--since` と同じ形式で指定できるが、date-only はローカル時刻の 00:00 とし `dayBoundary` を適用しない。`--since` / `--until` / `--project` と併用した場合は AND。`imported_at` はその session を最後に保存更新した JSONL ファイル処理の開始時刻（UTC・秒精度）であり、CLI 呼出し全体や最新の呼出しの時刻、本文差分時刻・commit 完了時刻・無重複消費を保証する watermark ではない。変更なしとしてスキップしたファイルは `imported_at` を更新しない。出力された `source` と `session_id` は `show --source` に渡して会話全体を再参照できる
- 時刻は `started_at ~ ended_at` の範囲形式で表示。ended_at がない場合は `started_at ~`。両方未知なら空欄
- `--since` または `--until` を指定した時は、NULL / 空 / 不正な started_at を一致させない。指定しない一覧では未知 timestamp も表示する
- `--project` は空でない `repo_path` への literal substring マッチ。`%`、`_`、`\` も文字列として扱う。値が config の alias グループに完全一致する場合はグループ全名に展開する（「設定ファイル」節参照）。NULL / 空の repository は条件に一致させない
- `repo_path` は絶対パスのため、`/` セグメントを跨いだ部分一致（例: `--project Sources/ryot`）も可能
- 表示は config の `projectAliases` に一致する場合は canonical 名のみ。一致しない場合、デフォルト表示は `repo_path` をそのまま
- `--short` は alias 非一致時に `filepath.Base(repo_path)`（ハイフン保持）
- 出力 TSV の列: `session_id`, `time_range`, `logical_day`, `project`, `custom_title`, `message_count`, `body_size`, `source`。source は `claude_code` / `codex` / `cursor_agent`
- TSV の `project` と `custom_title` はタブ・改行を空白に置換し、列と行の境界を保つ。JSON は生の文字列を出す
- `logical_day` は `ended_at`（無ければ `started_at`）をローカルタイムに変換し、その暦日の `dayBoundary` の境界時点より前なら前暦日、境界以降なら当暦日（`YYYY-MM-DD`）として出す。セッションを途中で分割せず、表示時に計算する
- `body_size` は非 sidechain メッセージの本文合計サイズ（UTF-8 バイト数）。show が出力する量の予測値として使う（show 前に大きいセッションかを判定する用途）。文字数でなくバイト数なのは、コンテキスト量の感覚と一致させるため。`message_count` は従来どおり sidechain を含む全行数
- `--format tsv|json`（デフォルト `tsv`）。JSON のフィールドは `source`, `sessionId`, `project`, `title`, `startedAt`, `endedAt`, `logicalDay`, `messageCount`, `bodySize`（共通仕様は「JSON 出力」節参照）

### プロジェクト一覧（projects）

- プロジェクト一覧をセッション数とともに表示
- `--since`/`--until` で時刻フィルタ（`started_at` 基準）。RFC3339 instant は `Z` または numeric offset で指定した正確な時点として解釈する。NULL / 空 / 不正な started_at は時刻条件に一致しない。date-only は従来どおりローカルタイムの 00:00 起点で、`dayBoundary` は適用しない
- SQL 側の集約キーは `repo_path` 一本。本体と worktree・サブディレクトリは取り込み時に同じ `repo_path` へ解決され、その保存値で集約される
- 出力 1 列目は config の `projectAliases` に一致する場合は canonical 名のみ。一致しない場合は `repo_path` そのもの
- TSV の project 名はタブ・改行を空白に置換し、列と行の境界を保つ。JSON は生の文字列を出す
- alias により同じ canonical 名になる行は cmd 層で session count を合算する
- `--short` は alias 非一致時に `filepath.Base(repo_path)`
- ソート: 直近セッション開始順（降順）
- `--format tsv|json`（デフォルト `tsv`）。JSON のフィールドは `project`, `sessionCount`

### 内容表示（show）

- セッション内容を Markdown で出力
- `show [--source <source>] <session-id>` は全 source を横断検索する。`--source` には search が出力する内部値 `claude_code` / `codex` / `cursor_agent` または CLI 表記 `claude-code` / `codex` / `cursor-agent` を指定でき、同じ `session_id` が複数 source に存在する場合に対象を選ぶ。未指定時は曖昧エラーとして候補を表示する。`all`、空値、未知値は不正で、`--source` は `--since` / `--until` の一括表示とは併用できない
- Markdown metadata は Session、Source、Project、Started。Started 行は `started_at ~ ended_at` の時刻範囲で、ended_at がない場合は `started_at ~`、両方未知なら空欄
- `--since`/`--until` で期間指定して一括表示（`started_at` 基準）。RFC3339 instant は `Z` または numeric offset で指定した正確な時点として解釈する。NULL / 空 / 不正な started_at は時刻条件に一致しない。date-only は `projects` と同じくローカルタイムの 00:00 起点で、`dayBoundary` は適用しない
- `--summary N` で各セッションの user メッセージから、除外後の先頭 N 件を表示。`0` または未指定で全文表示
- `--exclude-user-message-pattern <regex>` は繰り返し指定でき、trim 済みの本文全文に対して Go 正規表現を OR で評価する。1 回でも指定すると config の pattern 一覧全体を置き換える。照合は部分一致なので、先頭一致には `^` を明示する
- `--no-exclude-user-messages` は config の除外をこの呼び出しだけ無効化する。pattern 指定との併用はエラー。どちらの明示フラグも `--summary >= 1` が前提
- 除外 pattern は `outline` と `show --summary` の user message 表示だけに適用する。全文 `show`、`--turn`、`--tail`、`search`、保存 DB には適用せず、`sessions` の一覧行も除外しない。未設定時は除外なしで、slash prefix や合成 `/clear`・caveat の固定除外もない
- `--turn N` / `--turn N..M` で指定ターンだけ表示（両端含む）。1 ターンは user メッセージとそれに続く非 user メッセージ（assistant 応答等）。ターン番号は outline と同一の採番（GetMessages の全メッセージ列に対する採番）を共有する。範囲がセッションのターン数を超える場合は本文なしでセッションヘッダのみ出力し exit 0（エラーにしない）。`--turn ""`（空文字）は不正値としてエラー
- `--tail N` で末尾 N ターンだけ表示
- `--turn` と `--tail` は互いに排他。どちらも `--summary` とは併用不可
- `--turn` / `--tail` は `--since`/`--until` の一括表示モードでも各セッションに適用される
- メタデータ `Project` 行は config の `projectAliases` に一致する場合は canonical 名のみ。一致しない場合は `repo_path` をそのまま表示
- `--short` は alias 非一致時に `filepath.Base(repo_path)`
- `--project` は sessions と同じフィルタ規則（`repo_path` への substring マッチ、alias 展開含む）
- `--format markdown|json`（デフォルト `markdown`）。JSON はセッションの配列で、各要素は `source`, `sessionId`, `project`, `title`, `startedAt`, `endedAt`, `messages`（`role`, `content`, `timestamp` の配列）。単一セッション指定でも要素 1 の配列で出す（消費側のパースを一本化するため）。`--summary` / `--turn` / `--tail` のフィルタは `messages` にそのまま反映される

### アウトライン表示（outline）

- `outline <session-id>` で、セッションの user メッセージだけを「ターン番号・時刻・本文合計サイズ・先頭 1 行」の TSV で時系列表示する。長いセッションを全文 show する前に構造を掴む用途
- ターン番号は 1 始まり。sidechain を除いたメッセージ列を時系列に走査し、user メッセージごとに 1 増える（sidechain 除外は show と同じで、採番にも含めない）。最初の user メッセージより前のメッセージはターン 1 に畳み込む
- `/clear` エコーや `<local-command-caveat>` などの合成 user メッセージも turn 採番に数える。除外 pattern に一致すれば表示だけを省き、後続の turn 番号は元の値を保つ
- メッセージの時系列順は `timestamp` 昇順、同値は挿入順（rowid）で決定的に並べる（旧形式 Codex rollout は全レコードが同一 timestamp になるため、タイブレーカーがないと採番が実行ごとに揺れる）
- セッション ID の解決は show と同じ（`--source` で選択可能。未指定で複数 source に一致する場合は曖昧エラーで候補を表示）
- 時刻はローカルタイム `2006-01-02 15:04` 形式
- 出力 TSV の列: `turn`, `time`, `body_size`, `first_line`
- `body_size` はその turn に属する非 sidechain メッセージ本文の合計サイズ（UTF-8 バイト数）。`show --turn` で読む範囲の重さを見積もるため、user メッセージだけでなくその turn の assistant 応答等も含む。スキーマや import 結果には保存せず、表示時に `GetMessages` 結果から計算する
- 先頭 1 行は、前後の空白を除去した本文の最初の行。タブ・改行は空白に置換（TSV 保全）。切り詰めは行わない
- `--format tsv|json`（デフォルト `tsv`）。JSON のフィールドは `turn`, `timestamp`, `bodySize`, `firstLine`（`firstLine` は TSV と同じ先頭 1 行抽出だが、タブ・改行の空白置換は行わない）

### 検索（search）

- `search <query> [--since] [--until] [--day-boundary] [--project] [--limit N] [--offset M] [--format tsv|json]` で全メッセージ本文を横断検索する。デフォルトは `tsv`
- 実装は LIKE 全走査。FTS5 は日本語だと trigram 必須で索引が本文の 2〜3 倍に膨らみ、3 文字未満のクエリが索引で引けないため、LIKE で困るスケールになるまで見送り（本文 42 MB の DB で実測 0.1 秒前後）
- マッチは SQLite LIKE 準拠: 大文字小文字の無視は ASCII のみ。query の `%`、`_`、`\` は文字列として扱う
- sidechain メッセージは除外（show と同じ扱い）
- 出力 TSV の列: `session_id`, `turn`, `time`, `project`, `snippet`, `source`。source は `claude_code` / `codex` / `cursor_agent`。新しい順（メッセージ `timestamp` 降順、同値は rowid 降順）
- `turn` は outline / show --turn と同じ採番。ヒットしたメッセージが属する turn 番号を出すため、検索結果の `source` と `session_id` を `somniloq show --source <source> --turn <N> <session_id>` または `somniloq outline --source <source> <session_id>` に渡して再参照できる。source を省略して同じ `session_id` が複数 source にある場合は show の曖昧エラーに従う
- `time` はローカルタイム `2006-01-02 15:04` 形式
- `project` は config の `projectAliases` に一致する場合は canonical 名のみ。一致しない場合は `repo_path` をそのまま
- snippet はマッチの前後各 40 文字（rune 単位）。前後が切れている場合は `...` を付加。前後の空白は trim し、タブ・改行は空白に置換（TSV 保全）
- JSON のフィールドは `source`, `sessionId`, `turn`, `timestamp`, `project`, `snippet`。`timestamp` は DB 保存値、`snippet` はタブ・改行を置換しない生値（共通仕様は「JSON 出力」節参照）
- `--since`/`--until` は**メッセージの timestamp 基準**。RFC3339 instant は `Z` または numeric offset で指定した正確な時点として解釈する。NULL / 空 / 不正な timestamp は時刻条件に一致しない。sessions / show のセッション開始基準とは異なる（検索対象がメッセージのため。`docs/decisions/0013-search-time-filter-on-message-timestamp.md` 参照）。date-only（`YYYY-MM-DD`）は `dayBoundary`（未設定時 `00:00`、`--day-boundary HH:MM` で上書き可）を起点に解釈する。相対時刻と日時は `dayBoundary` の影響を受けない
- `--project` は sessions と同じフィルタ規則（`repo_path` への substring マッチ、alias 展開含む）
- `--limit N` は最大 N 件を返す。未指定時は無制限、N は 1 以上。`--offset M` は順序付け済みの先頭 M 件を飛ばす。未指定時は 0、M は 0 以上。すべての既存 filter と新しい順（timestamp 降順、同値は rowid 降順）を適用した後にページ化する
- 同じ query・filter・`--limit` で `--offset` を増やせば続きのページを取得できる。ただし、この保証は DB が固定で、相対時刻 filter を含む場合は解決済みの時刻条件も固定である場合だけ。DB の変更や snapshot はサポートしない

### JSON 出力（--format json）

機械消費（スクリプト・skill からの利用）向けの構造化出力。判断の経緯は `docs/decisions/0012-json-output-schema.md` 参照。

- 対象コマンド: `sessions` / `projects` / `outline` / `search`（`--format tsv|json`、デフォルト `tsv`）、`show`（`--format markdown|json`、デフォルト `markdown`）
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
  },
  "excludeUserMessagePatterns": ["^/", "^<command-name>/clear</command-name>"],
  "dayBoundary": "04:00"
}
```

- `projectAliases` は「現行名（canonical） → 旧名の配列」のマップ
- `--project` の値がグループの canonical 名または旧名のいずれかに**完全一致**したとき、グループ内の全名称に展開し、いずれかに substring マッチするセッションを対象にする（OR 条件）。完全一致以外は従来どおり値をそのまま 1 パターンとして使う
- 展開は双方向: 旧名を指定しても新名を指定しても同じグループに解決される
- ある名前が複数グループに重複して現れるような定義は検証しない。どのグループで展開・表示されるかは不定（alias の走査は map 順のため実行ごとに変わりうる）。実用上 1 リポジトリ 1 グループで足りるため
- `--project` フィルタ展開の対象は `--project` を持つコマンド（`sessions` / `show` / `search`）
- 表示正規化の対象は project 名を出すコマンド（`sessions` / `show` / `projects` / `search`）。alias グループに一致する `repo_path` / basename は canonical 名だけで表示し、旧名や元のパスを追加フィールドとして出さない
- `projects` 一覧では、alias により同じ canonical 名になる行を cmd 層で合算する。DB の `repo_path` は書き換えない
- `excludeUserMessagePatterns` は `outline` と `show --summary` で表示から除外する user message の Go 正規表現リスト。本文を trim した全文に対する部分一致で OR 評価し、未設定または空配列なら除外しない。不正な pattern は config 読み込みエラーとし、CLI override や一時無効化で隠さない
- 両コマンドで `--exclude-user-message-pattern <regex>` を繰り返し指定できる。指定があれば config の一覧を置換する。`--no-exclude-user-messages` は config の除外を呼び出し単位で無効化し、pattern override とは併用できない。空 regex は全本文に一致する有効な pattern で、無効化の意味には使わない
- `dayBoundary` は論理日の開始時刻をローカル時計の `HH:MM` で指定する。DST 切り替え日も指定したローカル時刻を使い、date-only の `--since` は指定暦日の境界を包含下限、`--until` は翌暦日の境界を排他上限とする。`logical_day` も同じ境界時点と比較する。欠落・重複するローカル時刻は Go の `time.Date` による解決に従う。未指定時は `00:00`。不正値は config 読み込みエラー。`sessions` / `search` の date-only `--since`/`--until` と `sessions` の `logical_day` 表示だけに使い、DB に焼き込まない

### v1.0.0 の設定・出力移行

- `commandPatterns` は自動移行しない。旧設定を `excludeUserMessagePatterns` へ手動で移すと、適用先は `sessions` のスキップ用ヒントから `outline` と `show --summary` の表示除外へ変わる。slash prefix の除外が必要なら `^/` を明示する
- 旧 `show --summary` の `/clear` 除外は `^<command-name>/clear</command-name>`、caveat 除外は `^<local-command-caveat>` として設定する。たとえば両方を除外する場合は次の通り:

```json
{
  "excludeUserMessagePatterns": [
    "^<command-name>/clear</command-name>",
    "^<local-command-caveat>"
  ]
}
```

- `--include-clear` は廃止した。特定の summary 呼び出しだけ全除外を止める場合は `show --summary N --no-exclude-user-messages` を使う
- sessions TSV は 10 列から 8 列になり、`source` は最終列（8 列目）へ移る。JSON の `nonCommandUserTurnCount` と `firstNonCommandUserLine` も削除した
- この公開変更は v1.0.0 の対象。`dayBoundary`、`logicalDay`、summary 機能は引き続き利用できる

## CLI インターフェース

```bash
somniloq import                          # Claude Code / Codex / Cursor Agent の JSONL を差分取り込み
somniloq import --source claude-code     # Claude Code の JSONL だけを差分取り込み
somniloq import --source codex           # Codex の rollout JSONL だけを差分取り込み
somniloq import --source cursor-agent    # Cursor Agent の transcript JSONL だけを差分取り込み
somniloq import --full                   # 全件再取り込み（確認あり）
somniloq import --full --yes             # 確認なしで全件再取り込み
somniloq sessions                        # セッション一覧
somniloq sessions --since 24h            # 直近24時間
somniloq sessions --imported-since 24h   # 直近24時間に保存更新されたセッション
somniloq sessions --since 2026-03-28     # 3/28 以降
somniloq sessions --until 2026-03-28     # 3/28 終わりまで
somniloq sessions --since 2026-03-28 --day-boundary 04:00  # 3/28 04:00 以降
somniloq sessions --since 7d --until 2h  # 直近7日間から最新2時間を除外
somniloq sessions --project Brimday      # プロジェクト名フィルタ
somniloq sessions --short                # プロジェクト名を短縮表示
somniloq show <session-id>               # セッション内容を Markdown で出力
somniloq show --source codex <session-id> # source を指定してセッション内容を出力
somniloq show --since 24h                # 直近24時間の全セッション
somniloq show --since 2026-03-28 --until 2026-03-29  # 3/28 の全セッション
somniloq show --summary 1 --since 24h                # 直近24時間の各セッションの冒頭 1 件
somniloq show --summary 3 --since 24h                # 冒頭 3 件
somniloq show --summary 1 --exclude-user-message-pattern '^<command-name>/clear</command-name>' --since 24h # /clear を除外
somniloq show --summary 1 --no-exclude-user-messages --since 24h # この呼び出しだけ除外を無効化
somniloq show --since 24h --short                    # プロジェクト名を短縮表示
somniloq show --turn 40..60 <session-id>             # ターン 40〜60 だけ表示
somniloq show --tail 3 <session-id>                  # 末尾 3 ターンだけ表示
somniloq outline <session-id>            # user メッセージをターン番号・時刻・本文サイズ・先頭1行で一覧
somniloq outline --source cursor_agent <session-id> # search の source で対象を選択
somniloq outline --exclude-user-message-pattern '^/' <session-id> # slash command を表示から除外
somniloq sessions --format json          # セッション一覧を JSON で出力
somniloq show --format json <session-id> # セッション内容を JSON で出力（outline / projects も --format json 対応）
somniloq search "auth bug"               # 全メッセージ本文を横断検索
somniloq search --format json "auth bug" # JSON で検索結果を出力
somniloq search --limit 50 --offset 50 "auth bug" # 51 件目から次の 50 件
somniloq search --since 2026-03-28 --day-boundary 04:00 "auth"  # 3/28 04:00 以降のメッセージ
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

主キー設計と Codex 対応 migration の判断の経緯は `docs/decisions/0004-codex-schema-and-migration.md` 参照。

```sql
-- セッション単位のメタデータ
CREATE TABLE sessions (
    source TEXT NOT NULL,         -- 'claude_code', 'codex', or 'cursor_agent'
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

-- 取り込み状態の追跡。主キーは jsonl_path 単独。Claude Code、Codex、Cursor Agent は
-- ベースディレクトリ（~/.claude/projects/、~/.codex/sessions/、~/.cursor/projects/）が分離して
-- いるため絶対パスだけで一意に特定でき、source は補助情報として保持する。
CREATE TABLE import_state (
    jsonl_path TEXT PRIMARY KEY,  -- JSONL ファイルの絶対パス
    source TEXT NOT NULL,         -- 'claude_code', 'codex', or 'cursor_agent'
    file_size INTEGER,            -- 最終取り込み時のファイルサイズ
    last_offset INTEGER,          -- 最終取り込み行のバイトオフセット
    imported_at TEXT NOT NULL
);
```

## Known limitations

- Claude Code が将来 `cwd` 空の `user`/`assistant` レコードを生成する仕様になった場合、somniloq 側ではそのまま `repo_path` 空で保存する。`projects` 集約で複数リポジトリが空グループに潰れる（`GROUP BY repo_path` 一本のため）。その時点で対応方針を再検討する
- alias グループに一致する project を JSON で出す場合、または `--short` を付けた場合、出力からは生の `repo_path` を取れない（表示名だけが出る）。生パスが必要になったら別フィールドの追加を検討する

## 互換性

- v0.12.0 以降、`search` query と `--project` は `%`、`_`、`\` を wildcard ではなく文字列として扱う。従来 wildcard を渡していた検索結果は変わる。explicit wildcard mode は提供しない。保存形式は変わらないため、DB migration、再 import は不要

## スキーマ変更への対応方針

- Go の struct タグで既知フィールドのみデコードし、未知は無視（デフォルト挙動）
- `version` フィールドを保存しておけば、問題発生時にバージョン別の切り分けが可能
