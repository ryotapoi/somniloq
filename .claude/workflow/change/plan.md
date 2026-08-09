# Plan Workflow

正本は `~/.config/agents/workflow/change/plan.md`。これを Read し、以下の Claude ハーネス固有規則とプロジェクト固有のレビュー観点を加えて実行する。

## Claude adapter

- 通常リスクの一般レビューは `/claude-code-built-in-review high`、High-risk は `/claude-code-built-in-review xhigh` として plan に記録する。
- Plan Review が必要な場合は、plan と関連資料だけを渡す `reviewer` subagent を `Agent` で起動する。model は `fable`、effort は `reviewer` 定義の `high` とし、`run_in_background: false` で結果を受け取る。

## Project-specific review

<!-- slot: 領域固有レビュー skill があれば追記する（例: UI 層を触るなら対応する specialist skill）。 -->
  <!-- /slot -->
