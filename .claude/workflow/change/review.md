# Review Workflow

正本は `~/.config/agents/workflow/change/review.md`。これを Read し、以下の Claude ハーネス固有規則とプロジェクト固有のレビュー観点を加えて実行する。

## Claude adapter

- Gatekeeper は `reviewer`、coordinator と finder は `scout` として fresh に起動する。モデルは `.claude/workflow/models.md` の役割既定に従い、finder は通常 Review finder、判断の重い観点だけ Judgment-heavy review finder の既定を使う。差し戻し後は同じ Gatekeeper subagent を再開する。
- Standard は `/claude-code-built-in-review high`、High-risk は `/claude-code-built-in-review xhigh` を使う。
- `Agent` は `run_in_background: false` で起動し、結果を起動呼び出しの戻り値で受け取る。background になった、または結果が返らない場合は完了通知を待たず `SendMessage` で回収する。

## Project-specific review

<!-- slot: 足す領域固有レビュー観点があれば追記する（例: UI 層に触れるなら対応する specialist skill）。 -->
  <!-- /slot -->
