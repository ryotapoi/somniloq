# Change Workflow

正本は `~/.config/agents/workflow/change/workflow.md`。これを Read し、以下の Codex ハーネス固有規則を加えて実行する。

## Codex adapter

- phase の論理名は `.agents/workflow/change/` 配下の同名 wrapper、`design-decision-record.md` は `.agents/workflow/design-decision-record.md` に解決する。
- 複数ファイル横断・キーワードのファンアウト調査は `scout`、実装は `worker`、Gatekeeper は `reviewer` を使う。初回を会話履歴なしで起動する時は `fork_turns: "none"` を明示し、差し戻し後の Gatekeeper は同じ subagent を再開する。
- running agent には追加連絡し、completed / idle agent は同じ agent を再開する。具体的な待機・中断・終了確認は `conductor.md` の Codex adapter に従う。
