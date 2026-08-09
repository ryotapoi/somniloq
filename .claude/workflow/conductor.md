# Conductor Workflow

正本は `~/.config/agents/workflow/conductor.md`。これを Read し、以下の Claude ハーネス固有の起動・監視規則を加えて実行する。

## Claude transport

- Goal の Orchestrator は Change ごとに fresh な Herdr pane の Conductor を起動する。
- `workers:` は Implementer / Gatekeeper の系統を選ぶ。未指定または `workers: claude` は Claude fresh subagent、`workers: codex` は `codex exec` とし、Change の途中で替えない。
- Claude Implementer は write-capable な `worker`、Gatekeeper は `reviewer` を `Agent` で起動する。モデルは `.claude/workflow/models.md` に従い、初回は fresh に起動し、差し戻し後の Gatekeeper は同じ subagent を再開する。
- Arbiter は `running-fresh-claude` に依頼し、共通 prompt と対象事実を渡す。起動不能なら別経路や別モデルで代行しない。
- 直接 Change の `auditors:` が未指定なら、`.claude/workflow/models.md` の Auditor defaults 全体を表の順序どおり選ぶ。Goal の brief から非空の指定を受け取った場合は置き換えず、直接 Change の明示指定には既定を追加せず指定順を保つ。選択した順序付きリストは要素数を仮定せず、Change 上限時に `~/.config/agents/workflow/auditor.md` と同 models の Goal Review / Auditor family launchers に渡す。

## Claude progress adapter

- 共通 recovery の status request は `SendMessage`、停止は `TaskStop`、同じ agent の再開は `SendMessage` に対応させる。
- `Agent` は同期実行を基本とし、background になった場合は完了通知を待たず `SendMessage` で結果または状態を回収する。

## `workers: codex`

- Implementer / Gatekeeper は `codex exec` で起動し、モデル・effort・sandbox を `.claude/workflow/models.md` に従って明示する。呼び出しには bounded time limit も明示する。
- sandbox による拒否や起動失敗の exit をテスト失敗（red）と解釈しない。実行不能として原因を分けて報告する。
- 出力ヘッダーの session ID を役割と対にして保持する。ID を確認できない場合は resume せず fresh 起動する。
- 同じ worker の再開は session ID 明示の `resume` とし、モデル・effort・sandbox を再指定する。`SendMessage`、status request、`TaskStop` に相当するAPIがないため、時間制限後にプロセス終了を確認してから recovery を判断する。
- full test suite は Conductor が foreground で実行する。Gatekeeper が実行環境の制約により再実行できない検証は Conductor が代行し、共通証拠形式で Gatekeeper に渡して代行した事実を報告する。

## Claude stop conditions

- `workers:` または明示モデルを利用可能な起動経路へ解決できない。
- worker または Arbiter を起動できない。Auditor の解決・起動・回収失敗は即時中断にせず、共通 Auditor 実行契約を終えてから既存の停止経路へ進む。
