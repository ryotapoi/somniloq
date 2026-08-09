# Change Workflow

正本は `~/.config/agents/workflow/change/workflow.md`。これを Read し、以下の Claude ハーネス固有規則を加えて実行する。

## Claude adapter

- phase の論理名は `.claude/workflow/change/` 配下の同名 wrapper、`design-decision-record.md` は `.claude/workflow/design-decision-record.md` に解決する。
- 複数ファイル横断・キーワードのファンアウト調査は Explore subagent に委譲する。ファイル1〜2個で済む確認は main が直接読む。
- subagent は結果を起動呼び出しの戻り値で受け取る同期実行を基本とする。`Agent` を使う場合は `run_in_background: false` を指定する。background になった、または結果が返らない場合は完了通知を待たず、`SendMessage` で結果または状態を問い合わせる。
- no-op の Monitor や sleep だけの待機ループを作らない。
