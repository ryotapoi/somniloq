# JSONL データソース仕様

Claude Code / Codex のセッション履歴ファイルの構造。

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
- ファイル名の stem は走査上の補助 ID として扱い、保存する `session_id` は `session_meta.payload.id` を使う

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

### 一意性と差分取り込み

- Codex の message レコードには Claude Code の `uuid` 相当が無いため、`messages.uuid` は `(rollout_path, line_number)` から決定的に生成する
- 差分取り込み時も、追記分を読む前にファイル先頭から offset 直前までの `session_meta` を読み直す。通常 `session_meta` はファイル先頭にあり、追記分だけを読むと session メタデータを失うため
