# Goal Workflow

正本は `~/.config/agents/workflow/goal.md`。これを Read し、以下の Codex ハーネス固有規則を加えて実行する。

## Codex crew and models

- この入口の crew は `codex` 固定。省略時も `crew: codex` とし、`claude` / `mixed` は受け付けない。Claude 系で回す Goal は Claude 側入口を使う。
- Conductor と worker の実モデル、reasoning effort、agent type は `.agents/workflow/models.md` を正とする。Implementer / Gatekeeper は `implementer: <短名>, gatekeeper: <短名>` と必要な effort で Goal 単位に指定し、途中で変えない。
- Goal Review reviewer は `reviewers: <短名>[, <短名>...]` で明示できる。明示がなければ共通既定を使い、選択と解決は `.agents/workflow/goal-review.md` に従う。
- Auditor は `auditors: <短名>[, <短名>...]` で明示できる。Goal 開始時に未指定なら `.agents/workflow/models.md` の Auditor defaults 全体を表の順序どおり選び、明示時は指定された短名だけを指定順で使って既定を追加しない。選択した順序付きリストは要素数を仮定せず Goal 全体で保持し、各 Change brief と Goal Review 上限時にそのまま使う。
- 削除・破壊的変更を含む、または影響範囲が読みにくく pane による逐次観測が必要な Goal は Claude 側入口を選ぶ。

## Codex transport

- Codex agent API は event の受信、状態の再確認、steer / interrupt を備えるため、Conductor の監督に別 pane を挟まない。
- Change ごとに `fork_turns: "none"` の fresh Codex subagent を Conductor として起動する。モデルと `reasoning_effort` を直接指定し、起動直後に指定どおり反映されたことを確認する。
- Conductor から Implementer / Gatekeeper への入れ子起動も Codex subagent を使う。この経路は実測済みであり、別プロセスの `codex exec` へ fallback しない。
- 状態は `list_agents` と `agent-liveness`、完了は mailbox update と報告ファイルで確認する。`wait_agent` の polling window は600秒とし、timeout は失敗や終了とみなさない。
- timeout 後は生存確認を取り直し、必要なら同じ Conductor へ message で status request を送る。応答も進捗もない場合だけ `interrupt_agent` を使い、終了確認後に repository state を調べる。
- Change 完了後は Conductor と子 worker の終了を確認し、次 Change は別の fresh Conductor で始める。
- commit の git 書き込み、および sandbox が原因か確定するための検証は必要に応じて sandbox 外で実行する。許可されない場合は commit 内容または残る検証制約を報告する。
- session log で裏取りする場合は `function_call` と `custom_tool_call` の両方を検索する。

## Codex Goal Review

- worker は担当 Change に必要な focused test までを実行する。全体 test suite は Conductor が commit 前に、Orchestrator が Goal 完了判定前に実行する。
- Goal Review は `.agents/workflow/goal-review.md`、実モデルは `.agents/workflow/models.md` を使う。上限時の Auditor は Goal で確定した `auditors:` と共通 Auditor 実行契約を使う。
- sandbox 外を要するフルスイートは Conductor が commit 前に、Orchestrator が commit 後にそれぞれ実行して green を確認する。

## Project-specific post-commit verification

<!-- slot: プロジェクト固有の追加照合手段があれば書く（format 差分ゼロの確認、app 側テストの実行条件など） -->
- `go build -o bin/somniloq ./cmd/somniloq` と `go vet ./...` を実行する。
<!-- /slot -->

## Codex stop conditions

- mailbox update を親へ配送できない agent API（Luna の `multi_agent_v1` を含む）しか利用できない。
- Conductor、子 worker、または Goal reviewer を指定どおり起動できない。Auditor の解決・起動・回収失敗は即時中断にせず、共通 Auditor 実行契約を終えてから既存の停止経路へ進む。
- model、reasoning effort、agent type、sandbox を実測可能な経路で保証できない。
