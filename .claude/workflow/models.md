# Model Definitions

モデル一覧、role defaults、effort、選択・検証方針の正本は `~/.config/agents/workflow/models.md`。これを Read し、以下を Claude 入口の transport adapter として使う。wrapper 単体に表がないことをモデル一覧の欠如と判定しない。

## Claude transport

- Claude subagent は `Agent` の `model` に短名を渡す。Implementer は write-capable な `worker`、Gatekeeper と Plan Review は Edit / Write 系 tool を禁止した汎用 `reviewer` を使う。
- Claude subagent の effort は per-call 指定できない。`worker` と `reviewer` の custom agent 定義は現在 `high` を固定し、明示された effort をその起動経路で保証できなければ停止する。
- Claude Conductor pane は `--model <短名> --effort <値>` で起動する。
- 単発 Change の `workers: codex` は `codex exec` を使い、モデル、effort、sandbox を明示する。Implementer は workspace-write、Gatekeeper は read-only とする。
- `codex exec resume <session-id> <CLI args> "<prompt>"` の順で指定し、`--last` は使わない。resume は sandbox を引き継がないため、`-c sandbox_mode=<read-only|workspace-write>` で再指定する。prompt を省略して stdin 待ちにしない。
- skill 経由ではこの file の短名を渡し、skill 側に実体名を写さない。
- Claude model の canonical 短名は `Agent --model` と `claude --model` にそのまま渡す。GPT model の canonical ID は `codex exec -m` に渡す。
- Claude 系 effort は custom agent 定義または起動元から継承し、pane では `--effort <value>` とする。GPT 系 effort は `-c model_reasoning_effort=<value>` とする。

## Auditor defaults

`auditors:` が未指定の場合は、次の短名を表の順序どおりリストとして使う。表の全行を選び、要素数を仮定しない。

| 順序 | 短名 |
|---|---|
| 1 | opus |

## Goal Review / Auditor family launchers

| family | launcher |
|---|---|
| Claude | `running-fresh-claude` |
| GPT | `running-fresh-codex` |

Goal Review と Auditor では共通 Model Catalog で短名から family を解決し、この表で launcher を解決して短名を渡す。未知の短名、family、launcher、または利用不能な launcher はエラーとして停止し、別値へ fallback しない。
