# Review Workflow

正本は `~/.config/agents/workflow/change/review.md`。これを Read し、以下の Codex ハーネス固有規則とプロジェクト固有のレビュー観点を加えて実行する。

## Codex adapter

- Gatekeeper は会話履歴を引き継がない fresh な `reviewer`、coordinator と finder は fresh `scout` とし、初回は `fork_turns: "none"` を明示する。差し戻し後は同じ Gatekeeper subagent を再開する。
- Standard は `$codex-built-in-review high`、High-risk は `$codex-built-in-review xhigh` を使う。
- nested coordinator 内の独立 finder は最大3体を同時起動し、それを超える観点は batch に分ける。Conductor → Gatekeeper → coordinator → finder までを通常経路とし、残りの agent depth は予備にする。
- running agent には追加連絡し、completed / idle agent は同じ agent を再開して結果を回収する。

## Project-specific review

<!-- slot: 足す領域固有レビュー観点があれば追記する（例: UI 層に触れるなら対応する specialist skill）。 --><!-- /slot -->
