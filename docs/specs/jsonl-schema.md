# JSONL データソース仕様

Claude Code / Codex / Cursor Agent のセッション履歴ファイルの構造。

## source 値

CLI の `--source` はユーザー向け表記として `all|claude-code|codex|cursor-agent` を受け取る。DB 内部の `sessions.source` / `messages.source` / `import_state.source` は `claude_code|codex|cursor_agent` を保存する。

## Claude Code

### ファイルの場所

`~/.claude/projects/<project-dir>/<session-id>.jsonl`

- project-dir: プロジェクトパスを `-` 区切りでエンコードしたもの（例: `-Users-ryota-Sources-ryotapoi-Brimday`）
- session-id: UUID v4（例: `a8171355-f84f-48e5-b27c-9e15c00da934`）

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

#### response_item.payload の保存対象

- `payload.type == "message"`
- `payload.role in ("user", "assistant")`
- `payload.content` は配列。`type` が `input_text`, `output_text`, `text` の要素の `text` を抽出し、複数あれば空行区切りで結合する
- `function_call`, `function_call_output`, `reasoning`, `event_msg` 等は保存しない
- レコード自身が `timestamp` を持たない場合は `session_meta` の `timestamp` を使う。per-record timestamp を持たない旧形式の rollout では、結果として同一セッションの全メッセージが同じ timestamp になる

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
- session identity は Cursor source と path 内の `<session-id>` の組である。

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
