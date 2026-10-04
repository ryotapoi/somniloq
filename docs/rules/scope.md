# Scope

本書が CLI 仕様・コマンド挙動・スキーマの正。README.md / README.ja.md は本書の派生ビューなので、本書のこれらの記述を変更したら README 両方を同期する。

v0.14.0 の [確定契約](../specs/v0.14.0-contract.md) のうち、TOML 設定・複数入力・Codex・Claude Code の本人と直接親の保存・各本人会話の完全 REF と専用 migrate は利用できる。まとまりの解決、新 search/show の原文・ページ仕様は後続実装。以下は現在利用できる CLI の仕様。

## 主要機能

### 取り込み（import）

source（DB 内部値は `claude_code` / `codex` / `cursor_agent`）ごとに専用の adapter で取り込む。共通の正規化スキーマ（`sessions` / `messages`）に保存する点は共通だが、ファイル配置・レコード形式・差分検出キーは source ごとに異なる。

`somniloq import --config NAME_OR_PATH` は設定の全入力を同じ SQLite DB に取り込む。`--input PATH` は繰り返し指定でき、その OR 条件と `--source all|claude-code|codex|cursor-agent` の交差で対象を選ぶ。input path は設定の root と同じ基準で正規化する。同じ source と実体 root の重複設定は一回だけ走査し、未存在 root は0件とする。

#### エラー処理と取り込みサマリ（source 共通）

- 取り込み終了時に `Imported <n> files (<scanned> scanned, <skipped> skipped, <failed> failed, <unparsed> unparsed lines)` を stdout に出力する
- parse / 正規化できない行（壊れた JSON、不正な payload）はスキップして続行し、`unparsed lines` に計上する。source が意図的に無視するレコード型（未知 type・非 message レコード・空行等）はカウントしない（`docs/decisions/0009-unparsed-line-visibility.md`）
- parse / 正規化失敗のうち先頭 5 件を `ファイル:行番号: エラー内容` 形式で stderr に出力する。この診断だけでは exit code を変更しない
- `unparsed lines` は「その実行で読んで解釈できなかった行数」を意味する。`import_state` の offset が進まないファイル（メタセッション等）の unparsed 行は、`import` のたびに繰り返し計上される
- ディレクトリ走査で読めないディレクトリは診断し、他入力の取り込みを続行する。Claude Code と Codex は本人全体を再構築するため、入力の走査・ファイル読み取りが不完全ならその入力の保存を中止し、旧本文・関係・差分状態を保持する。Cursor Agent のファイル単位の失敗はスキップして続行する（`docs/decisions/0010-non-fatal-scan-errors.md`）
- ディレクトリ走査・ファイル単位の取り込みエラーは stderr に列挙し、1 件以上あれば exit code は 1（取り込み自体は部分的に完了している）
- source のルートディレクトリが存在しない場合は、その source を未使用として扱いエラーにしない

#### Claude Code 用（`somniloq import --config NAME_OR_PATH --source claude-code`）

- 設定した Claude Code root（init の既定は `~/.claude/projects/`）を走査し、project 直下の root JSONL と `<root-session-id>/subagents/agent-<agent-id>.jsonl` を列挙
- 物理ファイル集合を再解析し、content hash と照合して変更された本人だけを原文順に再構築する。未変更本文の `imported_at` は更新しない。非本文の Agent/Task call と結果は毎回照合し、追記を跨ぐ結果・親後着・子後着・後着した競合を反映する
- root は `[sessionId]`、子・孫は path の `[rootSessionId,agentId]` を identity とする。子の会話 record の `agentId` は物理 path と一致する必要があり、不一致は unparsed として診断する。同じ sessionId・UUID でも本人や入力を混同しない
- 直接親は同じ物理親ファイル内の Agent/Task call id → tool_result.tool_use_id → record-level toolUseResult.agentId → 子 agentId の全リンクだけで確定する。重複する同一親の根拠は一つ、複数親・循環は未確定として診断する。path の root 所属は別に保存し、本文・parentUuid・時刻・prompt 類似から親を補わない
- 本人の sidechain、最初の prompt、text block 境界・空白・元 timestamp を保持し、非空本文だけに物理順の1始まり番号を付ける。fork-context-ref の未取得本文は仮造しない
- 各 JSONL を行単位で読み、`type` でフィルタ
- `user`/`assistant` → messages テーブルへ（text 部分のみ抽出）
- `user`/`assistant` レコードが初出のときだけ `sessions` 行を作成する。text 抽出結果が空の会話レコード（`tool_use` のみ・添付のみ・空白のみ）では、text 非空判定の前に session を保存するため `messages` 0 件の session が残る
- メタセッション（`custom-title` / `agent-name` 単独で `user`/`assistant` を持たない）は DB に保存しない。当該ファイルの `import_state` も進めず、後で会話レコードが追記されたときに先頭から再読み込みできる状態を維持する
- `user`/`assistant` の `cwd` から `repo_path` を解決して sessions に保存。`cwd` は会話レコードでは通常非空のため、会話セッションでは `repo_path` も通常非空（`ResolveRepoPath` 手順 4 で `cwd` 自体を返すため、`cwd` 非空なら必ず解決される）
- `custom-title` / `agent-name` レコードは、ファイル走査終了時点で対応する `sessions` 行が存在するときのみ反映する
- `import_state` を更新
- `--full` フラグで選択入力を再取り込み（確認プロンプトあり、デフォルト No）
  - `--yes` で確認をスキップ
  - 非対話環境（パイプ、CI 等）では `--yes` が必須
  - Claude Code は正常なファイル集合の解析後、選択入力の本文・会話・関係・差分状態を一つの transaction で置換し、保存失敗・不完全走査では前回保存を削除しない。他入力は保持し、選択0件では削除しない

#### repository の解決と既存データ

Claude Code と Codex は共通の `ResolveRepoPath` で `cwd` を解決する。空 cwd は空、`/.claude/worktrees/` を含む cwd は最初の marker より前を優先する。それ以外は Git の top-level と worktree 情報を使い、実在する通常の linked worktree とそのサブディレクトリも本体 repository の root に集約する。通常 repository と submodule はそれぞれ自身の root を使い、Git が解決できない cwd は元の値を保持する。消失した一般 worktree の本体は推測しない。

保存済みの非 NULL `repo_path` は自動補正されず、不変ファイルは差分 import でスキップされる。元ログと対象 worktree が残っていれば `somniloq import --config NAME_OR_PATH --full --yes` で選択入力を再構築できる。再構築する入力の元ログが残っていることを確認する。

通常 import/read は既存旧 schema（revision 0）や非対応 schema/revision を変更せず拒否する。既知の旧形式は専用 `migrate` で別 DB へ移せる。read コマンドは未存在 DB や parent directory を作成しない。

#### Codex 用（`somniloq import --config NAME_OR_PATH --source codex`）

- 設定した Codex root（init の既定は `~/.codex/sessions/`）配下の日付ディレクトリを再帰走査し、rollout JSONL を列挙
- 各 JSONL を物理行順に読み、`response_item` かつ `payload.type == "message"` かつ `role in ("user", "assistant")` の text 本文を対象とする。`input_text` / `output_text` / `text` block の文字列を配列順・空白込みで保持し、表示用本文は空行で連結する。空白のみ・tool-only は発言に数えない
- 最初の有効な `session_meta.payload.id` を本人 identity とし、本人の `cwd`、`git.branch`、`cli_version` を後続の埋込み親 metadata で上書きしない。`cwd` の `repo_path` 解決は Claude Code と共通。明示 `source.subagent.thread_spawn.parent_thread_id` は同入力の直接親参照として保存する。親が後着しても本人 REF と本文を変えず、別入力の同名 ID へ接続しない
- `subagent_history_start_ordinal` の明示境界がある場合だけ、境界未満の本文を継承 context として区別し、本人原文・発言番号・活動日時・本文検索に混ぜない。境界があるのに本文 ordinal が欠落した場合は所属不明として診断・保持し、本人へ推測分類しない。境界がなければ ordinal や最初の user から継承を推測しない。本人の sidechain 本文も一律除外しない
- 本人原文は同じ本人に属する rollout の入力内相対 path UTF-8 byte 辞書順、次に物理行順で `messageNumber` を1から付ける。payload.id のある本文は同じ ID・role・blocks・元 timestamp の完全一致だけを重複除外し、不一致は競合として既存 group と cursor を保護する。ID のない本文は物理的出自ごとに保持する。前方 path の追加・追記・途中編集では同本人全体を再構築して番号を確定する
- 発言日時は当該 record 自身の元 timestamp だけを保持する。欠落・空・不正値は比較上未知とし、metadata・mtime・import 時刻で補完しない。変更のない再 import は重複や `imported_at` の更新を生まない。差分再開と `--full` は同じ最終ファイル集合から同じ本人原文列を得る

#### Cursor Agent 用（`somniloq import --config NAME_OR_PATH --source cursor-agent`）

- 設定した Cursor Agent root（init の既定は `~/.cursor/projects/`）配下の `<project-slug>/agent-transcripts/<session-id>/<session-id>.jsonl` だけを取り込む
- `user` / `assistant` の `message.content` から `text` block だけを配列順に空行で連結し、tool、turn、未知正常 record、空行は保存しない
- ログにない timestamp、cwd、repository、version、title、usage、parent は補完しない
- `messages.uuid` の一意性は source、path、物理行に基づく。差分取り込み時も空行・無視行・unparsed 行を含む物理行番号を維持する
- 差分取り込み・`--full` 等のオプション体系は `import` と揃える

### 旧履歴の移行（migrate）

`somniloq migrate --config NAME_OR_PATH --from PATH` で、既知の旧形式 `legacy-v013` の固定 standalone snapshot を設定の `db` へコピーする。両フラグ必須、位置引数なし。移行先は未存在または revision 0 のユーザー object のない空 DB とし、設定には移行先 DB と残存 Codex ログの全入力を指定する。元と先の同一実体、元の sidecar、未知形状は拒否する。元 snapshot は変更しない。

初回コピーを一つの transaction で確定し、入力不明の旧履歴を legacy namespace で保持する。その後、設定の全 Codex 入力から物理行の UUID 出自と本人帰属を別々に確認し、本人 group の全文保存と同じ transaction で証明できた旧行だけを除く。未解析・競合・読み取りや保存失敗では旧行・前回正常会話・cursor を保持し、独立 group は続行する。ログ欠落、所属不明、Claude Code / Cursor Agent の旧履歴を保持する。

同じ snapshot の全 bytes digest と完了 receipt が一致する移行先だけ再実行できる。通常新 DB は再実行先として受理しない。stdout は JSON summary、stderr は本文を含まない診断。成功0、コピーや置換の失敗1、引数・設定・非対応 schema は2。所属不明の旧同名行を残した場合も置換不成功として終了1、単なるログ欠落保持は失敗にしない。詳細な受理条件・summary・保持条件は [移行契約](../specs/v0.14.0-migration.md) を参照する。

### 時刻フィルタの共通規則

- 保存済み timestamp の NULL・空文字列・RFC3339 として解釈できない値は、比較上は時点不明として扱う。範囲 filter には一致せず、filter なしでは行・本文を保持する。本人の本文取得順は発言番号であり、timestamp の有無や SQLite rowid で変えない。時刻を基準とする一覧・検索の並びは各コマンド節に従う。不正な非空文字列は保存・JSON 出力・時刻表示にそのまま残し、時刻を補完しない。ただし sessions TSV の時刻欄はタブ・改行を空白化する。CLI の不正な時刻引数はエラーとする。
- RFC3339 instant 入力の小数秒はミリ秒へ切り捨てず、ナノ秒精度で保持して時点として比較し、offset 表現が異なっても同じ時点なら等しい。`--since` は包含下限、`--until` は排他上限

### セッション一覧（sessions）

- セッション一覧を表示
- `--since`/`--until` で時刻フィルタ（相対: `24h`, `7d`、ローカル絶対値: `2026-03-28`, `2026-03-28T15:00`、RFC3339 instant: `2026-03-28T15:00:00Z`, `2026-03-29T00:00:00+09:00`）。絶対日付と分精度日時はローカルタイム。RFC3339 instant は `Z` または numeric offset で指定した正確な時点として解釈する。date-only（`YYYY-MM-DD`）は `dayBoundary`（未設定時 `00:00`、`--day-boundary HH:MM` で上書き可）を起点に解釈する。相対時刻と日時は `dayBoundary` の影響を受けない。出力のタイムスタンプもローカルタイム（`2006-01-02 15:04` 形式）
- `--imported-since` は session の `imported_at` を基準にした包含下限。相対時刻、ローカル日付、分精度日時、RFC3339 instant は `--since` と同じ形式で指定できるが、date-only はローカル時刻の 00:00 とし `dayBoundary` を適用しない。`--since` / `--until` / `--project` と併用した場合は AND。`imported_at` はその session を最後に保存更新した import 実行の開始時刻（UTC・秒精度）で、同じ実行の成功保存会話は共通の値を使う。本文差分時刻・commit 完了時刻・無重複消費を保証する watermark ではない。変更なしとしてスキップしたファイルは `imported_at` を更新しない。出力された完全 `ref` は `show` に渡して会話全体を再参照できる
- 時刻は `started_at ~ ended_at` の範囲形式で表示。ended_at がない場合は `started_at ~`。両方未知なら空欄
- `--since` または `--until` を指定した時は、NULL / 空 / 不正な started_at を一致させない。指定しない一覧では未知 timestamp も表示する
- `--project` は空でない `repo_path` への literal substring マッチ。`%`、`_`、`\` も文字列として扱う。値が config の alias グループに完全一致する場合はグループ全名に展開する（「設定ファイル」節参照）。NULL / 空の repository は条件に一致させない
- `repo_path` は絶対パスのため、`/` セグメントを跨いだ部分一致（例: `--project Sources/ryot`）も可能
- 表示は config の `projectAliases` に一致する場合は canonical 名のみ。一致しない場合、デフォルト表示は `repo_path` をそのまま
- `--short` は alias 非一致時に `filepath.Base(repo_path)`（ハイフン保持）
- 出力 TSV の列: `ref`, `time_range`, `logical_day`, `project`, `custom_title`, `message_count`, `body_size`, `source`。source は `claude_code` / `codex` / `cursor_agent`
- TSV の `time_range`、`project`、`custom_title` はタブ・改行を空白に置換し、列と行の境界を保つ。JSON は生の文字列を出す
- `logical_day` は `ended_at`（無ければ `started_at`）をローカルタイムに変換し、その暦日の `dayBoundary` の境界時点より前なら前暦日、境界以降なら当暦日（`YYYY-MM-DD`）として出す。セッションを途中で分割せず、表示時に計算する
- `body_size` は show 対象となる本人本文の合計サイズ（UTF-8 バイト数）。Claude Code・Codex は本人 sidechain を含み、Cursor Agent は従来の sidechain 除外に従う。show 前に大きいセッションかを判定する用途で、文字数でなくバイト数なのはコンテキスト量の感覚と一致させるため。`message_count` は本人本文の保存行数
- `--format tsv|json`（デフォルト `tsv`）。JSON のフィールドは `ref`, `source`, `sessionId`, `project`, `title`, `startedAt`, `endedAt`, `logicalDay`, `messageCount`, `bodySize`（共通仕様は「JSON 出力」節参照）

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
- `show [--source <source>] REF` は完全 REF の本人会話を選ぶ。裸 ID・短縮 REF・不存在 REF は exit 2。`--source` は DB 内部値または CLI 表記を受け取り、REF の source と一致する場合だけ利用できる。`all`、空値、未知値は不正で、期間による一括表示とは併用できない。
- Markdown metadata は Session、Source、Project、Started。Started 行は `started_at ~ ended_at` の時刻範囲で、ended_at がない場合は `started_at ~`、両方未知なら空欄
- `--since`/`--until` で期間指定して一括表示（`started_at` 基準）。RFC3339 instant は `Z` または numeric offset で指定した正確な時点として解釈する。NULL / 空 / 不正な started_at は時刻条件に一致しない。date-only は `projects` と同じくローカルタイムの 00:00 起点で、`dayBoundary` は適用しない
- `--summary N` で各セッションの user メッセージから、除外後の先頭 N 件を表示。`0` または未指定で全文表示
- `--exclude-user-message-pattern <regex>` は繰り返し指定でき、trim 済みの本文全文に対して Go 正規表現を OR で評価する。pattern はこの呼び出しにだけ適用する。照合は部分一致なので、先頭一致には `^` を明示する
- `--no-exclude-user-messages` は除外なしを明示する。pattern 指定との併用はエラー。どちらの明示フラグも `--summary >= 1` が前提
- 除外 pattern は `outline` と `show --summary` の user message 表示だけに適用する。全文 `show`、`--turn`、`--tail`、`search`、保存 DB には適用せず、`sessions` の一覧行も除外しない。pattern 未指定時は除外なしで、slash prefix や合成 `/clear`・caveat の固定除外もない
- `--turn N` / `--turn N..M` で指定ターンだけ表示（両端含む）。1 ターンは user メッセージとそれに続く非 user メッセージ（assistant 応答等）。ターン番号は outline と同一の採番（GetMessages の全メッセージ列に対する採番）を共有する。範囲がセッションのターン数を超える場合は本文なしでセッションヘッダのみ出力し exit 0（エラーにしない）。`--turn ""`（空文字）は不正値としてエラー
- `--tail N` で末尾 N ターンだけ表示
- `--turn` と `--tail` は互いに排他。どちらも `--summary` とは併用不可
- `--turn` / `--tail` は `--since`/`--until` の一括表示モードでも各セッションに適用される
- メタデータ `Project` 行は config の `projectAliases` に一致する場合は canonical 名のみ。一致しない場合は `repo_path` をそのまま表示
- `--short` は alias 非一致時に `filepath.Base(repo_path)`
- `--project` は sessions と同じフィルタ規則（`repo_path` への substring マッチ、alias 展開含む）
- `--format markdown|json`（デフォルト `markdown`）。JSON はセッションの配列で、各要素は `ref`, `source`, `sessionId`, `project`, `title`, `startedAt`, `endedAt`, `messages`（`role`, `content`, `timestamp` の配列）。単一セッション指定でも要素 1 の配列で出す（消費側のパースを一本化するため）。`--summary` / `--turn` / `--tail` のフィルタは `messages` にそのまま反映される

### アウトライン表示（outline）

- `outline REF` で、セッションの user メッセージだけを「ターン番号・時刻・本文合計サイズ・先頭 1 行」の TSV で時系列表示する。長いセッションを全文 show する前に構造を掴む用途
- ターン番号は 1 始まり。show 対象の本人メッセージ列を発言番号順に走査し、user メッセージごとに 1 増える。Claude Code・Codex は本人 sidechain を含み、Cursor Agent は従来の sidechain 除外に従う。最初の user メッセージより前のメッセージはターン 1 に畳み込む
- `/clear` エコーや `<local-command-caveat>` などの合成 user メッセージも turn 採番に数える。除外 pattern に一致すれば表示だけを省き、後続の turn 番号は元の値を保つ
- 本人メッセージは保存済み発言番号順に取得する。outline と旧 `--turn` の番号はその列の user 発言から計算し、発言番号とは異なる
- 会話の選択は show と同じ完全 REF。`--source` を付ける場合は REF の source と一致しなければならない
- 時刻はローカルタイム `2006-01-02 15:04` 形式
- 出力 TSV の列: `turn`, `time`, `body_size`, `first_line`
- `body_size` はその turn に属する show 対象の本人メッセージ本文の合計サイズ（UTF-8 バイト数）。`show --turn` で読む範囲の重さを見積もるため、user メッセージだけでなくその turn の assistant 応答等も含む。スキーマや import 結果には保存せず、表示時に `GetMessages` 結果から計算する
- 先頭 1 行は、前後の空白を除去した本文の最初の行。タブ・改行は空白に置換（TSV 保全）。切り詰めは行わない
- `--format tsv|json`（デフォルト `tsv`）。JSON のフィールドは `turn`, `timestamp`, `bodySize`, `firstLine`（`firstLine` は TSV と同じ先頭 1 行抽出だが、タブ・改行の空白置換は行わない）

### 検索（search）

- `search --config NAME_OR_PATH [--since <time>] [--until <time>] [--day-boundary <HH:MM>] [--project <name>] [--limit <n>] [--offset <n>] [--format <fmt>] <query>` で全メッセージ本文を横断検索する。デフォルトは `tsv`。フラグは検索語より前に置く
- 実装は LIKE 全走査。FTS5 は日本語だと trigram 必須で索引が本文の 2〜3 倍に膨らみ、3 文字未満のクエリが索引で引けないため、LIKE で困るスケールになるまで見送り（本文 42 MB の DB で実測 0.1 秒前後）
- マッチは SQLite LIKE 準拠: 大文字小文字の無視は ASCII のみ。query の `%`、`_`、`\` は文字列として扱う
- 継承 context と所属不明本文は検索しない。Claude Code・Codex の本人 sidechain は対象とし、Cursor Agent は従来の sidechain 除外に従う（show と同じ扱い）
- 出力 TSV の列: `ref`, `turn`, `time`, `project`, `snippet`, `source`。source は `claude_code` / `codex` / `cursor_agent`。新しい順（メッセージ `timestamp` 降順、同値は rowid 降順）
- `turn` は outline / show --turn と同じ採番。ヒットしたメッセージが属する turn 番号を出すため、検索結果の完全 `ref` を `somniloq show --config NAME_OR_PATH --turn <N> REF` または `somniloq outline --config NAME_OR_PATH REF` に渡して再参照できる
- `time` はローカルタイム `2006-01-02 15:04` 形式
- `project` は config の `projectAliases` に一致する場合は canonical 名のみ。一致しない場合は `repo_path` をそのまま
- snippet はマッチの前後各 40 文字（rune 単位）。前後が切れている場合は `...` を付加。前後の空白は trim し、タブ・改行は空白に置換（TSV 保全）
- JSON のフィールドは `ref`, `source`, `sessionId`, `turn`, `timestamp`, `project`, `snippet`。`timestamp` は DB 保存値、`snippet` はタブ・改行を置換しない生値（共通仕様は「JSON 出力」節参照）
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

全 DB コマンド（import / migrate / sessions / projects / search / show / outline）は `--config NAME_OR_PATH` が必須。コマンド名の前でも後でも指定できる。help / version / config init は設定不要。通常コマンドの `--db` は廃止。未指定・欠落時は exit 2、stderr に不足項目と `Run somniloq config init, then use --config default.` を表示し、DB・設定を自動生成しない。旧 JSON 設定は探索・変換しない。

`config init [NAME] [--output PATH] [--db PATH]` は設定だけを作り、DB は開かない。NAME 省略時は default、名前は `[A-Za-z0-9_-]+`。既定出力は `~/.somniloq/config/NAME.toml`、既定 DB は `~/.somniloq/NAME.db`。任意出力でも NAME が既定 DB を決める。parent directory は作成し、通常ファイル・directory・symlink（dangling を含む）への上書きは exit 2 で拒否する。stdout は作成した設定の絶対 path 一行。

```toml
db = "~/.somniloq/default.db"
dayBoundary = "00:00"

[projectAliases]
somniloq = ["Brimday"]

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

- init は上記3入力を生成する。`dayBoundary` と `projectAliases` は任意（既定 `00:00` と空）。`inputs.name` は任意の表示文字列で、DB に保存せず入力 identity に使わない。
- 全キーは上記だけ。未知キー・型違い・空 db/inputs/root・未知 source・不正 dayBoundary を拒否する。旧 `excludeUserMessagePatterns` は受理しない。表示除外が必要な現行 outline / summary では CLI の `--exclude-user-message-pattern` を使う。
- `--config` が名前規則を満たせば設定名、それ以外は path。`foo.toml` は path、拡張子なし相対ファイルは `./foo` と指定する。設定 path 自体の相対解決は実行 cwd 基準。
- 設定 symlink の実体親を相対 db/root の基準とする。先頭 `~` または `~/` だけを home へ展開し（`~other`・環境変数は展開しない）、絶対化・Clean・既存 symlink 解決を行う。未存在末尾は存在する最長 parent を実体解決して接続する。大文字小文字は変換しない。
- `projectAliases` は canonical 名から旧名配列への map。`--project` がグループ内の名に完全一致した場合だけ全名称へ OR 展開し、その他は literal substring。canonical/alias が別グループと重なる設定は拒否する。
- project 表示は一致する canonical 名を使う。projects は表示名ごとに件数を合算し、DB の repo_path は変更しない。
- `dayBoundary` はローカル時計の `HH:MM`。sessions/search の date-only フィルタと sessions の logical_day に適用し、DB に保存しない。DST 時の欠落・重複時刻は Go の `time.Date` の解決に従う。

## CLI インターフェース

```bash
somniloq config init                               # default TOML を生成（DB は未作成）
somniloq config init archive --output ./archive.toml --db ./archive.db
somniloq migrate --config archive --from ./archive-snapshot.db
somniloq import --config default                   # 全設定入力を差分取り込み
somniloq import --config default --source codex     # Codex 入力だけ
somniloq import --config default --input /path/to/logs --full --yes
somniloq sessions --config default --since 7d
somniloq sessions --config default --imported-since 24h --format json
somniloq search --config default --project somniloq "auth bug"
somniloq search --config default --limit 50 --offset 50 "auth bug"
somniloq outline --config default REF              # 一覧/検索の完全 REF を使う
somniloq show --config default --turn 12..18 REF
somniloq show --config default --format json REF
somniloq show --config default --since 24h          # 現行の期間一括表示
somniloq projects --config default --short
somniloq --config ./archive.toml sessions
somniloq --version
```

## SQLite と入力・会話の識別

DB path は TOML の db で指定する。新 DB は revision 1（`PRAGMA user_version=1`）で作成する。通常経路で旧 DB を変更・移行しない。revision が1でも以前の root-only shape は無変更で拒否する。既知の旧形式は専用 `migrate` を使い、それ以外は元ログから新しい DB へ取り込み直す（対応済み shape の DB の再構築には `--full --yes` を利用できる）。

| 保存対象 | 入力境界 |
| --- | --- |
| inputs | source と canonical root を一意に登録し、整数 FK で参照する |
| sessions | input と source 固有の会話 identity で分離する |
| messages | input と本人 identity ごとに UUID を区切り、同入力の本人会話へ接続する |
| import_state | input と JSONL path ごとに差分再開状態を保持する |
| migration_origin | 新規 DB では空。専用移行の receipt は通常 import で作らない |
| legacy_sessions / legacy_messages | 固定 snapshot の旧会話・旧行を入力未知のまま保存し、正常入力とは分離する |

入力キーは `sha256(UTF8(DB source) + NUL + UTF8(canonical root))` の64桁小文字 hex。同じ source/root は再処理や name 変更でも同入力、root 移動は別入力。Codex・Cursor Agent・Claude Code root の会話 identity は HTML escape なし・空白なし JSON 配列 `[sessionId]`。

完全 REF は `slq1:INPUT_KEY:SOURCE:BASE64URL_IDENTITY`（padding なし RFC4648 URL alphabet）。path・name・DB rowid は含まない。同じ root/source/identity なら再 import・別 DB でも安定する。Codex は子本人の identity と明示親参照を保存し、各本人 REF で会話を選べる。Claude Code の子・孫は `[rootSessionId,agentId]` で個別 REF を発行・選択できる。まとまり・子孫を展開する query は後続実装。移行した旧会話は `slq1:legacy:<snapshot_sha256>:<source>:<base64url_legacy_session_id>` の完全 REF で選択でき、正常入力の同名会話と区別する。部分置換後も旧 REF と残行の番号を維持する。

## Known limitations

- Claude Code が将来 `cwd` 空の `user`/`assistant` レコードを生成する仕様になった場合、somniloq 側ではそのまま `repo_path` 空で保存する。`projects` 集約で複数リポジトリが空グループに潰れる（`GROUP BY repo_path` 一本のため）。その時点で対応方針を再検討する
- alias グループに一致する project を JSON で出す場合、または `--short` を付けた場合、出力からは生の `repo_path` を取れない（表示名だけが出る）。生パスが必要になったら別フィールドの追加を検討する

## 互換性

- v0.12.0 以降、`search` query と `--project` は `%`、`_`、`\` を wildcard ではなく文字列として扱う。従来 wildcard を渡していた検索結果は変わる。explicit wildcard mode は提供しない。保存形式は変わらないため、DB migration、再 import は不要

## スキーマ変更への対応方針

- Go の struct タグで既知フィールドのみデコードし、未知は無視（デフォルト挙動）
- `version` フィールドを保存しておけば、問題発生時にバージョン別の切り分けが可能
