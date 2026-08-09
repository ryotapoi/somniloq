# Investigate Workflow

正本は `~/.config/agents/workflow/change/investigate.md`。これを Read し、以下の Codex ハーネス固有規則とプロジェクト固有の確認手段を加えて調査する。

## Codex adapter

- 複数ファイル横断・広域 grep・独立した仮説検証は `scout` に委譲する。会話履歴を引き継がない調査では `fork_turns: "none"` を明示する。ファイル1〜2個で済む確認は main が直接読む。

## Project-specific verification

<!-- slot: コード確認以外に使いたい確認手段があれば記載する（例: Preview / アプリ起動 / 公式ドキュメント、CLI なら実行して挙動を見る、実機・外部連携はユーザー確認）。 -->
    - 机上で分からない CLI 挙動は `bin/somniloq <args>` で実行して stdout / stderr / 終了コードを見る。
    <!-- /slot -->
