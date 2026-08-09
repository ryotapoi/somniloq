# Plan Workflow

正本は `~/.config/agents/workflow/change/plan.md`。これを Read し、以下の Codex ハーネス固有規則とプロジェクト固有のレビュー観点を加えて実行する。

## Codex adapter

- 通常リスクの一般レビューは `$codex-built-in-review high`、High-risk は `$codex-built-in-review xhigh` として plan に記録する。
- Plan Review が必要な場合は、plan と関連資料だけを渡す `reviewer` を `fork_turns: "none"` で起動する。model は `sol` を共通 Model Catalog から解決し、reasoning effort は GPT 系のベンダー推奨既定を使う。

## Project-specific review

<!-- slot: 領域固有レビュー skill があれば追記する（例: UI 層を触るなら対応する specialist skill）。 -->
    <!-- /slot -->
