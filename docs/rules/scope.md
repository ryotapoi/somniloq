# Scope

本書が CLI 仕様・コマンド挙動・スキーマの正。README.md / README.ja.md は本書の派生ビューなので、本書のこれらの記述を変更したら README 両方を同期する。

v0.14.0 の [確定契約](../specs/v0.14.0-contract.md) のうち、TOML 設定・複数入力・Codex・Claude Code の本人と直接親の保存・各本人会話の完全 REF と専用 migrate は利用できる。共通 resolver による関係解決と、複数 REF・確定子孫・発言フィルタ・ページ・TSV/JSON による show 原文取得は利用できる。まとまり一覧と共通 regexp の全一致詳細 search は利用できる。活動日4 mode と取り込み日時による候補選択を利用できる。以下は現在利用できる CLI の仕様。

## 主要機能

### 取り込み（import）

source（DB 内部値は `claude_code` / `codex` / `cursor_agent`）ごとに専用の adapter で取り込む。共通の正規化スキーマ（`sessions` / `messages`）に保存する点は共通だが、ファイル配置・レコード形式・差分検出キーは source ごとに異なる。

`somniloq import [--config NAME_OR_PATH]` は設定の全入力を同じ SQLite DB に取り込む。`--input PATH` は繰り返し指定でき、その OR 条件と `--source all|claude-code|codex|cursor-agent` の交差で対象を選ぶ。input path は設定の root と同じ基準で正規化する。同じ source と実体 root の重複設定は一回だけ走査し、未存在 root は0件とする。

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

`somniloq migrate [--config NAME_OR_PATH] --from PATH` で、既知の旧形式 `legacy-v013` の固定 standalone snapshot を設定の `db` へコピーする。`--from` は必須、`--config` 省略時は default、位置引数なし。移行先は未存在または revision 0 のユーザー object のない空 DB とし、設定には移行先 DB と残存 Codex ログの全入力を指定する。元と先の同一実体、元の sidecar、未知形状は拒否する。元 snapshot は変更しない。

初回コピーを一つの transaction で確定し、入力不明の旧履歴を legacy namespace で保持する。その後、設定の全 Codex 入力から物理行の UUID 出自と本人帰属を別々に確認し、本人 group の全文保存と同じ transaction で証明できた旧行だけを除く。未解析・競合・読み取りや保存失敗では旧行・前回正常会話・cursor を保持し、独立 group は続行する。ログ欠落、所属不明、Claude Code / Cursor Agent の旧履歴を保持する。

同じ snapshot の全 bytes digest と完了 receipt が一致する移行先だけ再実行できる。通常新 DB は再実行先として受理しない。stdout は JSON summary、stderr は本文を含まない診断。成功0、コピーや置換の失敗1、引数・設定・非対応 schema は2。所属不明の旧同名行を残した場合も置換不成功として終了1、単なるログ欠落保持は失敗にしない。詳細な受理条件・summary・保持条件は [移行契約](../specs/v0.14.0-migration.md) を参照する。

### 時刻フィルタの共通規則

- 保存済み timestamp の NULL・空文字列・RFC3339 として解釈できない値は、比較上は時点不明として扱う。範囲 filter には一致せず、filter なしでは行・本文を保持する。本人の本文取得順は発言番号であり、timestamp の有無や SQLite rowid で変えない。時刻を基準とする一覧・検索の並びは各コマンド節に従う。不正な非空文字列は保存・JSON 出力・時刻表示にそのまま残し、時刻を補完しない。search/show TSV は時刻欄も可逆 escape する。CLI の不正な時刻引数はエラーとする。
- RFC3339 instant 入力の小数秒はミリ秒へ切り捨てず、ナノ秒精度で保持して時点として比較し、offset 表現が異なっても同じ時点なら等しい。`--since` は包含下限、`--until` は排他上限

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

`show [--config NAME_OR_PATH] REF... [--descendants] [--role user|assistant] [--messages A:B] [--since VALUE] [--until VALUE] [--day-boundary HH:MM] [--limit N] [--offset N] [--tail N] [--one-line] [--format tsv|json]`。フラグは REF の前後に置ける。

- 完全 REF は最低一つ必要。全 REF を同じ read snapshot で先に検証し、一件でも不正・不存在なら stdout 空の exit 2。裸 ID・短縮 REF は受理しない。
- REF の指定順に本人会話を選び、`--descendants` は確定子孫だけを親先行 DFS・兄弟 REF 辞書順で展開する。会話は初出だけを採用し、root 所属だけで直接親不明の子や祖先は含めない。各会話の保存済み発言番号順の列を連結する。
- role・発言番号・日時で絞り、ページ化してから一行化する。`--role` は user / assistant。`--messages A:B` は両端を含む正整数の範囲で、`A:` / `:B` も指定できる。空両端・逆順は不正。各会話へ同じ番号条件を適用し、元の `messageNumber` は振り直さない。
- `--since` / `--until` は発言自身の保存 timestamp の instant が基準で、下限包含・上限排他。日付 `YYYY-MM-DD` は設定の `dayBoundary`（既定 `00:00`、`--day-boundary HH:MM` で上書き）を起点にし、until 日付は翌日の境界までを含む。日付以外は zone 付き RFC3339 / RFC3339Nano のみ。相対時刻・zone なし日時は受理しない。未知・不正日時は期間なしなら保持し、期間ありなら不一致。session 開始日時で補完しない。
- 既定は全文（limit=null、offset=0）。`--limit N` / `--offset N` は非負整数で、全 filter 後の発言列を会話をまたいでページ化する。limit=0 は空ページ、末尾を超えた offset でも total を返す。
- `--tail N` は filter 後の全列末尾 N **発言**を元の順序で返す。limit / offset の明示とは 0 を含め併用できない。envelope は limit=N、offset=max(total-N,0)。tail=0 は空の末尾ページ。
- `--one-line` はページ確定後の text の最初の LF より前だけを表示し、CRLF の CR は除く。空白は trim せず、先頭 LF は空 text。blocks は原文のまま。
- 既定 TSV、JSON は発言単位の `{items,total,count,limit,offset,hasMore,nextOffset}` object。各発言は `ref,messageNumber,role,timestamp,text,blocks,parentRef,rootRef,provenance` を常に返す。欠落日時は null、不正な非空日時は保存値、復元不能な legacy blocks は null、既知の配列は空でも []。provenance は source_record / legacy_saved。
- total は filter 後・page 前、count は items 数。hasMore は残件の有無、nextOffset は続きがあれば offset+count、なければ null。limit=0 の hasMore は offset<total、nextOffset は null。件数・本文・関係は同じ read snapshot から取得する。
- TSV は先頭 `# page\t` の後に items を除く envelope の compact JSON、一行 header、その後は発言ごと一行。列順は上記 field 順。null は `\N`、配列は compact JSON、文字列は backslash→`\\`、tab→`\t`、LF→`\n`、CR→`\r` の順で可逆 escape する。
- 成功は0（0件を含む）、入力・設定・REF・非対応 DB は2、I/O 失敗は1。旧 `outline` command、show の `--summary` / `--turn` / 表示除外 / `--source` / `--project` / `--short` / Markdown と、REF なしの期間入口は提供しない。user の一覧は `--role user --one-line`、番号範囲は `--messages` で取得する。

### 検索（search）

- `search [--config NAME_OR_PATH]` は session 指定の有無で全一致詳細とまとまり一覧を選ぶ。デフォルトは `tsv`。フラグは位置 pattern/query より前に置く

#### 詳細検索（--session REF）

`search [--config NAME_OR_PATH] --session REF [-e PATTERN]... [-F] [--all] [--limit N] [--offset N] [--format tsv|json] [PATTERN]`。位置 PATTERN が先頭、その後は -e の指定順で pattern 列を作る。フラグは位置 PATTERN より前に置く。

- Go regexp の大小文字区別が既定、`(?i)` を受理し、`-F` は全 pattern を固定文字列にする。既定 OR、`--all` は対象本文集合全体で各 pattern が一度以上一致する AND。不成立は0箇所、成立時は全 pattern の一致を返す。空・無効・欠落 pattern は DB query 前に exit 2。
- `--session REF` は指定本人と確定直接親を辿る子孫だけを対象とする。祖先・兄弟・root 所属だけの子は含めない。完全 REF の不正・不存在は exit 2、stdout 空。入力・source を跨いで接続せず、ページ化前に絞る。本人 show と core の関係 resolver を共有する。
- 原文の UTF-8 byte `[startByte,endByte)` を返す。同一区間を統合して1始まり patternIndexesを残し、異なる重なりは別箇所。ゼロ幅と隣接空一致は Go FindAllStringIndex に従う。REF 辞書順→保存済み messageNumber→start/end 順で、occurrenceNumber は各発言内で1から採番し、page後も維持する。
- matchText は一致原文、lineText は一致の先頭・末尾を含む行全体。LF は直前の行に属し、非空一致の末尾は endByte-1 の行まで含む。ゼロ幅は startByte の行、本文末尾は末尾行（末尾 LF の後なら空行）。行末 LF 自体は lineText に含めないが複数行の途中 LF は保持する。
- JSON は `{items,total,count,limit,offset,hasMore,nextOffset}`。item は `ref,messageNumber,occurrenceNumber,role,timestamp,startByte,endByte,patternIndexes,matchText,lineText` を常に返す。日時は raw 値、未知は null。TSV は同じ列順、先頭 `# page` metadata、可逆 escape を show と共有する。
- 既定は全件（limit=null/offset=0）。明示 limit/offset だけ箇所単位で page 化する。limit=0 の hasMore は offset<total、nextOffset=null。末尾超過も page 前 total を返す。REF 解決・本文・件数は同じ read snapshot。
- 負の limit/offset、不正・不存在 REF、pattern エラーは stdout 空の exit 2。下記と同じ input/source/project 候補条件、発言 timestamp 条件を先に適用して AND を判定する。期間なしでは未知日時を保持する。日時入力は下記と共通で、詳細の --time-mode は active のみ受理する。started/last/overlap は一覧専用として exit 2 で拒否する。明示 active も期間指定を必要とする。

#### まとまり一覧（--session なし）

`search [--config NAME_OR_PATH] [PATTERN] [-e PATTERN...] [-F] [--all] [--input PATH...] [--source SOURCE...] [--project TEXT] [--since VALUE] [--until VALUE] [--time-mode active|started|last|overlap] [--imported-since RFC3339] [--day-boundary HH:MM] [--limit N] [--offset N] [--format tsv|json]`。フラグは位置 PATTERN より前に置く。pattern は省略でき、全候補を一覧にする。空・無効 pattern は exit 2。照合器と OR/AND の意味は詳細と共通で、親子に別 pattern があっても同じまとまり内の候補で AND を満たせる。

- input/source/project で候補本人を選び、その本文だけを照合して保存関係でまとまりに集約する。input/source は各 OR、条件種間は AND。source は `claude-code|codex|cursor-agent` の3種のみ、all は拒否。input は設定の実体親を基準に path を正規化し、DB の canonical root と照合する。
- project は repo_path の末尾名への大小文字区別 substring。alias 完全一致時だけ canonical と aliases へ OR 展開する。未知 project は不一致。親を候補から除くとその本文で AND を満たさない。
- 一行は同じ input/source 内のまとまり。Claude root 所属だけの子も members に含む。legacy と Cursor は独立。欠落親は key のみで member に数えない。ref は存在する root の REF、欠落 root では member REF の最小辞書順。
- JSON item は `ref,input,source,project,title,startedAt,lastAt,importedAt,members,matchedMembers,memberCount` の全 field。本文抜粋・旧 turn は返さない。input は canonical root、legacy は null。project/title は存在する root の保存値のみ、欠落 root は null。members は元の全まとまり、matchedMembers は候補に残った本人（実際の pattern 一致本人には狭めない）、memberCount は全 members 数。
- startedAt/lastAt は全 members の本人本文の既知日時の最小/最大。importedAt は全 members の最大を時点比較して raw 値を返す。未知日時は補完しない。lastAt 既知の降順→未知最後→まとまり key 辞書順。
- JSON は `{items,total,count,limit,offset,hasMore,nextOffset}`、既定全件（limit=null）。明示 `--limit N` だけが上限となり、`--offset` のみでは整列済み結果の残り全件を返す。全 filter 後・page 前のまとまり数が total、items 数が count。明示 limit=0 は空ページ、hasMore は offset<total、nextOffset は null。offset は0以上、末尾超過も total を返す。結果0件も items=[] で成功 exit 0。入力エラーは stdout 空で exit 2。
- TSV は先頭 `# page\t` の後に items を除く compact JSON、一行 header、item ごと一行。列順は上記 field 順。null は `\N`、配列は compact JSON、文字列は show/詳細と同じ可逆 escape。
- 件数・項目・関係・metadata は同じ read snapshot。照合前に limit を適用しない。DB 変更を跨ぐ別ページの固定は保証しない。
- `--since` / `--until` は日付または zone 付き RFC3339 / RFC3339Nano。相対時刻・zone なし日時・空値は exit 2。日付は設定 dayBoundary（既定00:00、CLI --day-boundary が上書き）を起点にし、until 日付は翌日の境界へ進める。下限包含・上限排他で、小数秒と offset の instant を保つ。since >= until、不正 mode、不正 dayBoundary を拒否する。
- `--time-mode` は active（既定）/started/last/overlap。明示 mode は期間指定を必要とする。active は候補の期間内実発言だけで OR/AND を判定し、pattern なしでも対象発言が必要。started/last は全 members の既知本人原文日時の最小/最大で期間選択し、候補全文を照合する。overlap は last >= since && start < until（未指定側は無限）で選び、期間内実発言なしでも跨ぐまとまりを含める。他 mode の照合では未知日時本文も含め、全日時未知のまとまりは期間に一致しない。期間なしでは未知日時本文・本文なし候補も落とさない。表示日時と整列は全 group で決める。
- `--imported-since` は zone 付き RFC3339 / RFC3339Nano の包含下限。各候補本人の importedAt >= 下限を input/source/project と AND で先に判定し、その候補本文だけを照合する。members・表示日時・importedAt は全まとまりのまま。子だけ一致しても一覧を返すが、除外された親本文で AND を満たさない。詳細にも同じ候補条件を適用する。

### JSON 出力（--format json）

機械消費（スクリプト・skill からの利用）向けの構造化出力。判断の経緯は `docs/decisions/0012-json-output-schema.md` 参照。

- 対象コマンド: `projects` / `search` / `show`（`--format tsv|json`、デフォルト `tsv`）
- show と search は envelope object、projects は JSON 配列。結果0件は envelope の items=[]、配列入口は `[]`。show と search の関係・本文・件数は同じ read transaction から取得する。
- フィールド名は camelCase
- タイムスタンプは DB 保存値をそのまま出す。show は元の offset・精度・不正な非空 raw 値も保持する。search/show の TSV でも元値を保持する（タイムゾーン情報を失わないため）
- 文字列は生値（TSV のタブ・改行置換はしない。エスケープは JSON 側で担保される）
- `title` は `custom_title` の生値（session_id フォールバックはしない）
- projects の `project` は alias canonical 表示と `--short` を反映した表示名（alias 一致時は canonical 名のみ、alias 非一致時のデフォルトは `repo_path` の生値）
- 不正な `--format` 値はエラー（`unknown format: ...`）。DB を開く前に検証する
- インデント 2 スペース、HTML エスケープ（`<` `>` `&` の `\uXXXX` 化）は無効

### 設定ファイル（config）

全 DB コマンド（import / migrate / projects / search / show）は `--config NAME_OR_PATH` の省略を `--config default` と同じ扱いにする。コマンド名の前でも後でも指定できる。help / version / config init は設定不要。通常コマンドの `--db` は廃止。設定欠落時は exit 2、stderr に不足項目と `Run somniloq config init, then use --config default.` を表示し、DB・設定を自動生成しない。旧 JSON 設定は探索・変換しない。

`config init [NAME] [--output PATH] [--db PATH]` は設定だけを作り、DB は開かない。NAME 省略時は default、名前は `[A-Za-z0-9_-]+`。既定出力は `~/.somniloq/config/NAME.toml`、既定 DB は `~/.somniloq/NAME.db`。任意出力でも NAME が既定 DB を決める。設定宛先と参照 DB の両方が未存在の場合だけ設定を生成する。いずれかに通常ファイル・directory・symlink（dangling を含む）があれば exit 2 で拒否し、既存内容を保持する。DB の存在検査は名前の既定 DB と明示 `--db` に同じ規則を適用し、相対 DB は設定の実体親を基準に解決する。設定の parent directory は作成する。stdout は作成した設定の絶対 path 一行。

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
- 全キーは上記だけ。未知キー・型違い・空 db/inputs/root・未知 source・不正 dayBoundary を拒否する。旧 `excludeUserMessagePatterns` は受理しない。show の表示除外フラグも提供しない。
- `--config` が名前規則を満たせば設定名、それ以外は path。`foo.toml` は path、拡張子なし相対ファイルは `./foo` と指定する。設定 path 自体の相対解決は実行 cwd 基準。
- 設定 symlink の実体親を相対 db/root の基準とする。先頭 `~` または `~/` だけを home へ展開し（`~other`・環境変数は展開しない）、絶対化・Clean・既存 symlink 解決を行う。未存在末尾は存在する最長 parent を実体解決して接続する。大文字小文字は変換しない。
- `projectAliases` は canonical 名から旧名配列への map。`--project` がグループ内の名に完全一致した場合だけ全名称へ OR 展開し、その他は literal substring。canonical/alias が別グループと重なる設定は拒否する。
- projects の表示は一致する canonical 名を使い、表示名ごとに件数を合算する。search は root の repo_path をそのまま表示する。DB の repo_path は変更しない。
- `dayBoundary` はローカル時計の `HH:MM`。search/show の date-only フィルタに適用し、DB に保存しない。DST 時の欠落・重複時刻は Go の `time.Date` の解決に従う。

## CLI インターフェース

```bash
somniloq config init                               # default TOML を生成（DB は未作成）
somniloq config init archive --output ./archive.toml --db ./archive.db
somniloq migrate --config archive --from ./archive-snapshot.db
somniloq import --config default                   # 全設定入力を差分取り込み
somniloq import --config default --source codex     # Codex 入力だけ
somniloq import --config default --input /path/to/logs --full --yes
somniloq search --config default --since 2026-10-01
somniloq search --config default --imported-since 2026-10-01T00:00:00+09:00 --format json
somniloq search --config default --project somniloq "auth bug"
somniloq search --config default --limit 50 --offset 50 "auth bug"
somniloq show --config default REF --role user --one-line
somniloq show --config default REF --messages 12:18
somniloq show --config default REF1 REF2 --since 2026-10-01 --until 2026-10-01 --format json
somniloq projects --config default --short
somniloq --config ./archive.toml search
somniloq --version
```

一覧から指定日の原文を取得する POSIX sh 例（条件に合うまとまり全件を選択。明示 --limit で上限を指定できる）:

```sh
sh <<'SH'
set -- $(somniloq search --config default --since 2026-10-01 --until 2026-10-01 --format json | jq -r '.items[].members[]' | sort -u)
if [ "$#" -gt 0 ]; then
  somniloq show --config default "$@" --since 2026-10-01 --until 2026-10-01 --format json
fi
SH
```

members は root 所属だけの子を含むまとまり全体、matchedMembers は候補本人。sort 順が show の会話順となり、空選択では show を呼ばない。

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

完全 REF は `slq1:INPUT_KEY:SOURCE:BASE64URL_IDENTITY`（padding なし RFC4648 URL alphabet）。path・name・DB rowid は含まない。同じ root/source/identity なら再 import・別 DB でも安定する。Codex は子本人の identity と明示親参照を保存し、各本人 REF で会話を選べる。Claude Code の子・孫は `[rootSessionId,agentId]` で個別 REF を発行・選択できる。まとまりの検索一覧は保存関係と共通 resolver で本人 REF を集約する。確定子孫の show 展開と search の `--session REF` は利用できる。移行した旧会話は `slq1:legacy:<snapshot_sha256>:<source>:<base64url_legacy_session_id>` の完全 REF で選択でき、正常入力の同名会話と区別する。部分置換後も旧 REF と残行の番号を維持する。

## Known limitations

- Claude Code が将来 `cwd` 空の `user`/`assistant` レコードを生成する仕様になった場合、somniloq 側ではそのまま `repo_path` 空で保存する。`projects` 集約で複数リポジトリが空グループに潰れる（`GROUP BY repo_path` 一本のため）。その時点で対応方針を再検討する
- alias グループに一致する project を JSON で出す場合、または `--short` を付けた場合、出力からは生の `repo_path` を取れない（表示名だけが出る）。生パスが必要になったら別フィールドの追加を検討する

## 互換性

- v0.12.0 以降、横断 `search` query と `--project` は `%`、`_`、`\` を wildcard ではなく文字列として扱う。従来 wildcard を渡していた検索結果は変わる。explicit wildcard mode は提供しない。保存形式は変わらないため、DB migration、再 import は不要

## スキーマ変更への対応方針

- Go の struct タグで既知フィールドのみデコードし、未知は無視（デフォルト挙動）
- `version` フィールドを保存しておけば、問題発生時にバージョン別の切り分けが可能
