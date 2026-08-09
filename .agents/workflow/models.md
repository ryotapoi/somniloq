# Model Definitions

モデル一覧、role defaults、effort、選択・検証方針の正本は `~/.config/agents/workflow/models.md`。これを Read し、以下を Codex 入口の transport adapter として使う。wrapper 単体に表がないことをモデル一覧の欠如と判定しない。

## Codex transport

- Conductor、Implementer、Gatekeeper は Codex subagent として起動し、`model` と `reasoning_effort` を直接指定できる。
- Implementer は write-capable な `worker`、Gatekeeper と Plan Review は `sandbox_mode = "read-only"` の汎用 `reviewer` を使う。`reviewer` 定義は model / effort を固定せず、起動時に共通 Model Catalog から解決した値を直接指定する。
- Goal / Change worker は GPT 系固定で、`codex exec` へ fallback しない。
- GPT model は共通一覧の `canonical ID` を `model` に、共通 effort 値を `reasoning_effort` に渡す。

## Auditor defaults

`auditors:` が未指定の場合は、次の短名を表の順序どおりリストとして使う。表の全行を選び、要素数を仮定しない。

| 順序 | 短名 |
|---|---|
| 1 | sol |

## Goal Review / Auditor family launchers

| family | launcher |
|---|---|
| Claude | `running-fresh-claude` |
| GPT | `running-fresh-codex` |

Goal Review と Auditor では共通 Model Catalog で短名から family を解決し、この表で launcher を解決して短名を渡す。未知の短名、family、launcher、または利用不能な launcher はエラーとして停止し、別値へ fallback しない。
