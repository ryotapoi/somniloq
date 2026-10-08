# CLI 公開形式

機械利用者が入力を識別し、検索結果から本人原文を再取得するための形式。コマンドの使い方は README と help を参照する。

## 入力と REF

source の CLI・設定値は `claude-code` / `codex` / `cursor-agent`、DB・JSON・REF で使う値は `claude_code` / `codex` / `cursor_agent` とする。入力キーは `sha256(UTF8(DB source) + NUL + UTF8(canonical root))` の64桁小文字hex。同じ source と実体 root の重複設定は一入力とする。root 移動は別入力、name変更は同入力。本文の類似による統合はしない。

source 固有 identity は空白なし JSON 配列の UTF8（JSON string escaping を用い HTML escape なし）。Codex/Cursor は `[sessionId]`、Claude root は `[sessionId]`、Claude 子・孫は `[rootSessionId,agentId]`。Claude は一つの root 内の agentId を使うため root と同じ sessionId を持つ子を潰さない。同じ identity を別入力では統合しない。

REF は `slq1:INPUT_KEY:SOURCE:BASE64URL_IDENTITY`。Base64 は padding なし RFC4648 URL alphabet。path・name・DB rowid をREFに露出しない。完全 REF のみ受理し、短縮・裸IDは受理しない（曖昧選択を防ぐ）。同入力の再処理・別DBで同じrootから構築した場合に安定する。root移動・identity変更でREFは変わる。未知入力の旧保存値は [legacy REF](migration.md) を使い、通常REFと混同しない。存在しないREFはexit 2。

## 出力

`search` と `show` は既定 TSV、JSON は UTF-8 object で、同じ envelope `{items,total,count,limit,offset,hasMore,nextOffset}` を使う。totalはfilter後page前、countはitems数、limitは無制限null、nextOffsetは続きがあればoffset+count、なければnull。limit=0ではhasMoreはoffset<total、nextOffset=null（進まないpageを自動反復しない）。末尾超過もtotalを返す。件数と items は同じ read transaction で取得する。

一覧既定は全件（limit=null）、明示limitだけが上限。offsetのみでは整列済み結果の残り全件を返す。last既知の降順→未知は最後→まとまりkey辞書順。詳細/showは既定無制限。詳細は会話REF辞書順→番号→箇所。showは指定した REF の順序。表示日時は既知raw値、不正rawはそのまま、未知null。復元不能のlegacy blocksはnull、既知の配列は空でも `[]`、booleanとintegerは型を保つ。`search` と `show` の JSON item は以下の全 field を常に持ち、省略しない。

| item | fields |
| --- | --- |
| 一覧 | ref, input（canonical rootまたはnull）, source, project（root値またはnull）, title（root値またはnull）, startedAt, lastAt, importedAt（membersの最大）, members（REF配列）, matchedMembers（REF配列）, memberCount |
| 一致箇所 | ref, messageNumber, occurrenceNumber, role, timestamp, startByte, endByte, patternIndexes, matchText, lineText |
| show発言 | ref, messageNumber, role, timestamp, text, blocks（連結前text配列）, parentRef（確定し存在する親またはnull）, rootRef（所属rootが存在する場合のREFまたはnull）, provenance（source_record または legacy_saved） |

`messageNumber` は本人原文の1始まり番号で `show` と詳細検索に共通する。`occurrenceNumber` は同一発言内の1始まり番号。`startByte` / `endByte` は原文 text 内の UTF-8 byte の半開区間 `[startByte,endByte)` で、`matchText` を切り出せる。`patternIndexes` は pattern 列の1始まり番号の配列。位置引数の pattern があれば先頭に置き、その後に `-e` を指定順で並べる。`-e` だけの場合も最初は1となる。

一覧project/titleは存在するrootの値のみ、子から推測しない。欠落rootではnull。`search` と `show` の TSV は先頭 `# page\t` の後に同envelopeのitemsを除くcompact JSON、一行header、その後itemごと一行。列順は上表field順。nullは `\N`、配列はcompact JSON、文字列はbackslash→`\\`、tab→`\t`、LF→`\n`、CR→`\r` の順にescapeして可逆にする。JSON一行textもtextだけ短縮、blocksは原文のまま。JSON/TSVは本文抜粋を一覧に載せない。

`search` と `show` は成功時に0（0件含む）、入力・設定・REF・非対応DBの拒否に2、I/O失敗に1を返す。診断はstderr、結果はstdoutに分ける。`migrate` の終了値とsummaryは [移行手順](migration.md#成功失敗と出力) に従う。表示用現在時刻を日時unknownに代入しない。

`projects --format json` は `{project,sessionCount}` の配列を返す。既定 TSV は header と page 行を持たず、`project` と `session_count` の2列を行ごとに返す。project 中の tab と改行は空白に置換する。

JSON は2スペース字下げで、HTML 文字を escape しない。
