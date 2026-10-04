# JSONL データソース仕様

Claude Code / Codex / Cursor Agent のセッション履歴ファイルの構造。

この文書の既存取り込み・保存記述は現行実装の契約。v0.14.0 の子孫・本人／継承・原文日時の実装予定契約と合成 fixture は [元ログ調査](v0.14.0-log-evidence.md) を参照する。以下の観測追記は現行 parser がすでに対応している意味ではない。

## source 値

`import` の CLI `--source` はユーザー向け表記として `all|claude-code|codex|cursor-agent` を受け取る。DB 内部の `sessions.source` / `messages.source` / `import_state.source` は `claude_code|codex|cursor_agent` を保存する。TOML inputs.source も CLI 表記3種を使う。`show` / `outline` は完全 REF で本人を選び、`--source` を付ける場合は REF の source に一致する DB 内部値または CLI 表記を指定する。`all` は受け取らない。

標準 path は config init が生成する既定 root。追加 root も同じ source adapter で走査する。source と canonical root で入力を識別し、同名 session/UUID と差分状態を入力間で混同しない。root 会話 identity と REF は [scope](../rules/scope.md#sqlite-と入力会話の識別) を参照する。

## 共通の末尾行と差分再開

末尾改行のない正常な JSON レコードは、その import で取り込む。末尾改行がなく unparsed となった行は診断に数えるが、差分再開位置をその行頭に残し、追記でファイルサイズが増えたら同じ物理行を再処理する。改行済みの unparsed 行では再開位置を進める。本文のない prefix は従来どおり保存境界を進めず、本文が現れたときに読み直す。

保存済みの過去の欠落は自動回復しない。対応 revision の DB は、元ログが残っていれば `import --config NAME_OR_PATH --full` で選択入力だけを再構築できる。他入力は保持する。旧形式 DB は通常経路で無変更拒否し、専用 migrate は後続実装。

## Claude Code

### ファイルの場所

`~/.claude/projects/<project-dir>/<session-id>.jsonl`

- project-dir: プロジェクトパスを `-` 区切りでエンコードしたもの（例: `-Users-ryota-Sources-ryotapoi-Brimday`）
- session-id: UUID v4（例: `a8171355-f84f-48e5-b27c-9e15c00da934`）

### 子孫ファイルの観測（v0.14.0 調査）

root JSONL と同じ project 配下の `<root-session-id>/subagents/agent-<agent-id>.jsonl` に子と孫が保存される。子本人は path の root session ID と agent ID の組で識別する。root 所属は直接親を意味しない。同一物理親ファイルの Agent/Task `tool_use.id` と `tool_result.tool_use_id`、同 record の `toolUseResult.agentId` と子の agent ID の厳密な一致を直接親の根拠にする。prompt の一致だけでは解決しない。`fork-context-ref` は未取得 context の参照であり、参照先本文を仮造しない。観測件数と未確定例は [調査根拠](v0.14.0-log-evidence.md#観測根拠) を参照する。

### レコード構造

各行が1つの JSON オブジェクト。`type` フィールドで種別が決まる。

#### 主要 type

| type | 内容 | 保存対象 |
|---|---|---|
| `user` | 人間の発話 + tool_result | text のみ |
| `assistant` | クロコの応答 + tool_use | text のみ |
| `system` | subtype: local_command, api_error, turn_duration, stop_hook_summary | 不要 |
| `progress` | ストリーミング中間データ（大量、レコードの過半数） | 不要 |
| `file-history-snapshot` | ファイルバックアップスナップショット | 不要 |
| `custom-title` | セッション名（`customTitle` フィールド） | メタデータ |
| `agent-name` | エージェント名（`agentName` フィールド） | メタデータ |
| `last-prompt` | 最後のプロンプト | 不要 |
| `queue-operation` | キュー操作 | 不要 |

#### user/assistant 共通フィールド（全バージョンで安定）

```
type, message, sessionId, cwd, timestamp, gitBranch, uuid, parentUuid, version, userType, isSidechain
```

- `user` / `assistant` の `sessionId` と `uuid` は非空値を必須とする。欠落・空文字列・`null` の行は unparsed とし、path・物理行・欠落フィールドを診断する。その行は本文の有無にかかわらず session / message を保存・更新せず、前後の正常行の取り込みは継続する。ID の推測生成は行わない。
- 既存 DB に保存済みの空 ID の session / message は自動修復しない。元のログが残っていれば `import --config NAME_OR_PATH --full` で再構築できる。選択入力の再構築については「共通の末尾行と差分再開」の注意に従う。

#### バージョンで増減するフィールド（v2.1.37〜v2.1.86 で確認）

出たり消えたりする。未知フィールドは無視する設計にすること。

- `isMeta`, `slug`, `permissionMode`, `todos`, `thinkingMetadata`
- `planContent`, `imagePasteIds`, `promptId`, `toolUseResult`
- `entrypoint`（v2.1.78〜）

#### message.content の構造

**user:**
- `string` — 素のテキスト入力
- `[]object` — tool_result 付き。各要素の `type` は `"text"` or `"tool_result"`

**assistant:**
- `[]object` — 各要素の `type` は `"text"` or `"tool_use"`
  - `tool_use`: `{type, id, name, input}` — name がツール名（Read, Edit, Bash, etc.）

## Codex

### ファイルの場所

`~/.codex/sessions/<yyyy>/<mm>/<dd>/rollout-*.jsonl`

- rollout ファイルは日付ディレクトリ配下にネストされるため、`~/.codex/sessions/` を再帰走査する
- 保存する `session_id` は `session_meta.payload.id` を使う

### レコード構造

各行が1つの JSON オブジェクト。トップレベルはおおむね `timestamp`, `type`, `payload`。

#### 主要 type

| type | 内容 | 保存対象 |
|---|---|---|
| `session_meta` | セッションメタデータ | `payload.id`, `payload.cwd`, `payload.cli_version`, `payload.git.branch` |
| `response_item` | モデル応答・ユーザー入力・tool call 等 | `payload.type == "message"` かつ `payload.role` が `user` / `assistant` の text のみ |
| `event_msg` | token count, task complete 等のイベント | 不要 |
| `turn_context` | turn ごとの実行コンテキスト | 不要 |

#### session_meta.payload の主なフィールド

```
id, timestamp, cwd, originator, cli_version, source, model_provider, git
```

- `cwd` から `ResolveRepoPath` で `repo_path` を解決する
- `git.branch` は存在する場合のみ `git_branch` に保存する
- `cli_version` は `version` に保存する
- `id` が欠落・空文字列・`null` の `session_meta` は unparsed とし、path・物理行・原因を診断する。有効な metadata がまだない rollout の後続本文は保存しない。差分取り込みの prefix 復元でも同じ検証を適用する。
- 既存 DB に保存済みの空 ID の session / message は自動修復しない。元のログが残っていれば `import --config NAME_OR_PATH --full` で再構築できる。選択入力の再構築については「共通の末尾行と差分再開」の注意に従う。

#### response_item.payload の保存対象

- `payload.type == "message"`
- `payload.role in ("user", "assistant")`
- `payload.content` は配列。`type` が `input_text`, `output_text`, `text` の要素の `text` を抽出し、複数あれば空行区切りで結合する
- `function_call`, `function_call_output`, `reasoning`, `event_msg` 等は保存しない
- レコード自身が `timestamp` を持たない場合は `session_meta` の `timestamp` を使う。per-record timestamp を持たない旧形式の rollout では、結果として同一セッションの全メッセージが同じ timestamp になる

### 本人と継承の観測（v0.14.0 調査）

子 rollout の最初の metadata が本人 ID、その後に埋め込まれた metadata が親 ID の例がある。最初の metadata の `source.subagent.thread_spawn.parent_thread_id` は直接親を示す。`session_meta.payload.subagent_history_start_ordinal` がある場合、本文 record の top-level `ordinal` が境界未満なら継承 context、境界以上なら本人。本人最初の本文が assistant の例を確認しており、最初の user を境界にしない。境界がない旧形式は ordinal の有無にかかわらず除外を推測しない。旧 UUID の物理行出自は本人帰属の証明とは別。具体的な入力と非推測の境界は [代表 fixture](../../internal/ingest/testdata/v0.14.0/README.md) を参照する。

### 一意性と差分取り込み

- Codex の message レコードには Claude Code の `uuid` 相当が無いため、`messages.uuid` は `(rollout_path, line_number)` から決定的に生成する
- 差分取り込み時も、追記分を読む前にファイル先頭から offset 直前までの `session_meta` を読み直す。通常 `session_meta` はファイル先頭にあり、追記分だけを読むと session メタデータを失うため

## Cursor Agent

Cursor Agent は CLI の `--source cursor-agent` で取り込む。source 未指定または `--source all` では
Claude Code と Codex とともに取り込む。ここでは Cursor Agent adapter が従う入力契約を定める。

### 観測事実と限界

Cursor Agent `2026.09.10-fd3934a` のローカルログ 607 files / 9259 records を観測した。全件で
次の path 構造、session directory と basename の一致、`role=user|assistant`、
`message.content` の text / tool_use、および top-level `type=turn_ended` を確認した。既存 prefix を
保った末尾追記も controlled follow-up で確認した。

一方、独立した timestamp、cwd、repository、version、title、usage、parent は観測されなかった。
project slug は全件を信頼して可逆復元できない。local format は公式契約ではなく version 依存であり、
形式変化が観測されたときに再調査する。以下の fixture は観測ログのコピーではなく、無害な架空値だけで
作った合成・匿名化済みの仕様検証例である。

### 受理するファイルと session

Cursor projects root からの相対 path が次と一致する JSONL だけを受理する。

```
<project-slug>/agent-transcripts/<session-id>/<session-id>.jsonl
```

- `<project-slug>` は session ID や repository metadata の代用にしない。
- `<session-id>` は空であってはならず、session directory 名と basename（拡張子を除く）が一致しなければならない。UUID version の制限は設けない。
- 任意の場所の `.jsonl`、空の session directory、directory と basename が不一致の path は受理しない。
- `private-tmp` も特別除外しない。観測した事実ではなく、除外根拠がないため上の一般規則を適用する設計判断である。
- session identity は入力内の `[sessionId]`（path 内の `<session-id>`）であり、別入力では同名でも別会話となる。

### レコードの取り込み

各 JSONL の物理行を順に扱う。JSON object の既知 role `user` / `assistant` では、
`message.content` 配列のうち `type` が `text` で `text` が文字列の block だけを配列順に取り出し、
複数なら空行（`\n\n`）で結合する。同じ行の複数 text は 1 message とし、同じ本文でも別の物理行なら別 message とする。

- `tool_use` など non-text block、`turn_ended`、未知の正常 record、空行は意図的無視であり、会話本文にしない。
- 空または text のない既知 role の正常 content は session 登録を許すが、既存 `PersistMessage` 契約に従い message を作らない。
- 壊れた JSON、既知 role の壊れた `message` / `content` envelope、または text が文字列でない block は unparsed とする。その行の部分的本文は保存しない。
- `<timestamp>`、`<user_query>`、`<cwd>` を含む text は通常本文として保存する。tag を除去せず、metadata にも抽出しない。これらの頻度や追加構造を観測したとは主張しない。

### metadata、順序、再処理

ログにない timestamp、cwd、repository、version、title、usage、parent は埋めない。既存の正規化 string
fields は空値、parent は nil とし、repository を保存するときの NULL 化は既存の永続化規則に従う。
mtime、import 時刻、slug、本文内 tag から事実として補完してはならない。

message identity は同一 source、path、物理行から決定的に導く。空行、無視行、unparsed 行も物理行番号に含める。
同じ入力の再処理は session / message の重複を作らず、件数と会話順序を変えない。timestamp を合成して順序を作らない。
hash algorithm と DB query の実装細部はこの契約では固定しない。

`internal/ingest/testdata/cursor-agent/README.md` はこの契約に対応する合成 fixture の行ごとの期待結果を示す。
