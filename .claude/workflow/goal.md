# Goal Workflow

正本は `~/.config/agents/workflow/goal.md`。これを Read し、以下の Claude ハーネス固有規則を加えて実行する。

## Claude crew and models

- この入口の crew は `claude` 固定。省略時も `crew: claude` とし、`codex` / `mixed` は受け付けない。GPT 系で回す Goal は Codex 側入口を使う。
- Conductor と worker の実モデル、effort、CLI 引数は `.claude/workflow/models.md` を正とする。Implementer / Gatekeeper は `implementer: <短名>, gatekeeper: <短名>` で Goal 単位に明示指定できる。Claude subagent の effort は custom agent 定義または起動元から継承し、明示値を保証できなければ停止する。
- Goal Review reviewer は `reviewers: <短名>[, <短名>...]` で明示できる。明示がなければ共通既定を使い、選択と解決は `.claude/workflow/goal-review.md` に従う。
- Auditor は `auditors: <短名>[, <短名>...]` で明示できる。Goal 開始時に未指定なら `.claude/workflow/models.md` の Auditor defaults 全体を表の順序どおり選び、明示時は指定された短名だけを指定順で使って既定を追加しない。選択した順序付きリストは要素数を仮定せず Goal 全体で保持し、各 Change brief と Goal Review 上限時にそのまま使う。
- 削除・破壊的変更を含む、または影響範囲が読みにくい Goal は、pane を実読して逸脱を止められるこの入口を優先する。

## Herdr transport

- supervisor には event の受信、状態の再確認、steer / interrupt が必要である。同期 subagent は親を block し、background subagent は入れ子の完了通知が欠落し得るため、Claude Conductor は独立 pane で起動する。この通知制約が解消された時だけ transport の再評価を行う。
- `test "${HERDR_ENV:-}" = 1` を開始条件とする。herdr 外なら pane 運転を始めず、herdr 内での起動を依頼して停止する。
- Change ごとに `herdr pane current` で自分を特定し、`herdr pane split --pane <self> --direction right --no-focus --cwd <absolute-project-path>` で隣に pane を作る。別 tab はユーザー指定時だけ作る。
- 起動直前の epoch 秒を記録し、`herdr agent start <unique-name> --kind claude --pane <id> -- <model-and-effort-args>` で fresh Conductor を起動する。短名を裸で渡さず、registry の model と effort の CLI 引数を両方使う。
- 起動直後に `herdr pane read` で model、effort、cwd、modal の有無を確認する。想定外の承認要求は承認せず停止する。
- brief は `herdr agent prompt <target> <text> --wait --timeout 600000` で投入する。wait の復帰だけを完了とせず、報告ファイルと対象 session ID の heartbeat `Stop` で確認する。event 名は完全一致で照合し、`SubagentStop` は完了判定に使わない。
- 未完了なら pane read と `agent-liveness` で状態を確認する。working なら `herdr agent wait <target> --timeout 600000` で待ち直し、blocked なら内容を実読して想定どおりの承認要求だけを扱う。`agent_prompt_stalled` や他 session の Stop を完了根拠にしない。
- 完了後は `/exit` を送って pane read で終了を確認し、`herdr pane close <id>` で閉じる。Enter が反映されない場合は再送する。次 Change へ pane を持ち越さない。

## Heartbeat

- pane 起動前に `.claude/settings.local.json` の PreToolUse / PostToolUse / Stop / SubagentStop が `~/dotfiles/workflow-canon/provision-heartbeat.py` の `claude_heartbeat_command()` と一致することを確認する。未設置または不一致なら Goal 中に書き換えず、同 script の実行をユーザーへ依頼して停止する。この入口では `.codex/hooks.json` を開始条件にしない。
- worktree を使う場合はその worktree 側にも hook を provision し、pane 起動前に同じ照合を行う。
- heartbeat は `tmp/workflow/heartbeat.jsonl` を使う。他の Goal session がないと確認できた場合だけ開始時に空にしてよい。完了判定は起動時刻以降に初出した対象 session ID で絞り、session ID を特定できなければ報告ファイルだけを根拠にする。
- 常設 hook は終了時も除去・復元しない。一時 heartbeat file は不要なら削除してよい。

## Claude Goal Review and report

- Goal Review は `.claude/workflow/goal-review.md`、実モデルは `.claude/workflow/models.md` を使う。上限時の Auditor は Goal で確定した `auditors:` と共通 Auditor 実行契約を使う。
- Final Report の直前に `reporting` skill を読み、共通 Final Report 契約へ加える。
- フルスイートは Conductor と Orchestrator が foreground で実行する。

## Project-specific post-commit verification

<!-- slot: プロジェクト固有の追加照合手段があれば書く（format 差分ゼロの確認、app 側テストの実行条件など） -->
- `go build -o bin/somniloq ./cmd/somniloq` と `go vet ./...` を実行する。
<!-- /slot -->

## Claude stop conditions

- Herdr、heartbeat、pane 状態、model / effort のいずれかを実測可能な方法で確認できない。
- pane の完了根拠がなく、working / blocked のどちらとも判定できない。
- Conductor または Goal reviewer を指定どおり起動できない。Auditor の解決・起動・回収失敗は即時中断にせず、共通 Auditor 実行契約を終えてから既存の停止経路へ進む。
