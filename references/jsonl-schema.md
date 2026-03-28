# JSONL データソース仕様

Claude Code のセッション履歴ファイルの構造。

## ファイルの場所

`~/.claude/projects/<project-dir>/<session-id>.jsonl`

- project-dir: プロジェクトパスを `-` 区切りでエンコードしたもの（例: `-Users-ryota-Sources-ryotapoi-Brimday`）
- session-id: UUID v4（例: `a8171355-f84f-48e5-b27c-9e15c00da934`）

## レコード構造

各行が1つの JSON オブジェクト。`type` フィールドで種別が決まる。

### 主要 type

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

### user/assistant 共通フィールド（全バージョンで安定）

```
type, message, sessionId, cwd, timestamp, gitBranch, uuid, parentUuid, version, userType, isSidechain
```

### バージョンで増減するフィールド（v2.1.37〜v2.1.86 で確認）

出たり消えたりする。未知フィールドは無視する設計にすること。

- `isMeta`, `slug`, `permissionMode`, `todos`, `thinkingMetadata`
- `planContent`, `imagePasteIds`, `promptId`, `toolUseResult`
- `entrypoint`（v2.1.78〜）

### message.content の構造

**user:**
- `string` — 素のテキスト入力
- `[]object` — tool_result 付き。各要素の `type` は `"text"` or `"tool_result"`

**assistant:**
- `[]object` — 各要素の `type` は `"text"` or `"tool_use"`
  - `tool_use`: `{type, id, name, input}` — name がツール名（Read, Edit, Bash, etc.）
