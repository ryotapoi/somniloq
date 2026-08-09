# Conductor Workflow

正本は `~/.config/agents/workflow/conductor.md`。これを Read し、以下の Codex ハーネス固有の起動・監視規則を加えて実行する。

## Codex transport

- Goal の Orchestrator は Change ごとに fresh Codex subagent の Conductor を起動する。
- Implementer / Gatekeeper は GPT 系固定とし、`workers:` は受け付けない。既定モデルと明示指定は `.agents/workflow/models.md` に従う。
- Implementer は `worker`、Gatekeeper は `reviewer` で起動する。model と `reasoning_effort` を直接指定し、初回は `fork_turns: "none"` で fresh にする。差し戻し後の Gatekeeper は同じ subagent を再開し、subagent を起動・再開できない場合は `codex exec` へ切り替えない。
- Gatekeeper が sandbox 制約により workspace 書き込みを伴う検証を再実行できない場合は Conductor が代行し、共通証拠形式で Gatekeeper に渡して代行した事実を報告する。
- commit の git 書き込みと、sandbox が原因か確定するための検証は必要に応じて sandbox 外で実行する。許可されない場合は commit 内容または残る検証制約を報告する。
- Arbiter は `running-fresh-codex` に依頼し、共通 prompt と対象事実を渡す。起動不能なら別経路や別モデルで代行しない。
- 直接 Change の `auditors:` が未指定なら、`.agents/workflow/models.md` の Auditor defaults 全体を表の順序どおり選ぶ。Goal の brief から非空の指定を受け取った場合は置き換えず、直接 Change の明示指定には既定を追加せず指定順を保つ。選択した順序付きリストは要素数を仮定せず、Change 上限時に `~/.config/agents/workflow/auditor.md` と同 models の Goal Review / Auditor family launchers に渡す。

## Codex progress adapter

- 共通 recovery の待機は `wait_agent`、status request と再開は同じ agent へのmessage、停止は interrupt に対応させる。
- 1回の polling window は600秒とする。status request にも応答しない場合だけ interruptし、終了確認後に repository state を確認する。

## Codex stop conditions

- mailbox update を親へ配送できない agent API しか利用できない。
- 明示された agent type、モデル、effort を実測可能な起動経路で保証できない。
- worker または Arbiter を起動できない。Auditor の解決・起動・回収失敗は即時中断にせず、共通 Auditor 実行契約を終えてから既存の停止経路へ進む。
