# 取り込み対象の JSONL 形式

外部 agent のログ形状と、本人・関係を示す明示フィールドを記録する。これらは source が提供する全バージョンの保証ではない。ローカルで観測した版差と件数は [時点付き記録](../cache/2026-10-05-log-evidence.md) に分ける。

## Claude Code

root は `<project-dir>/<session-id>.jsonl`、子は同じ project 内の `<root-session-id>/subagents/agent-<agent-id>.jsonl`。子の path は root 所属を示し、直接親を示さない。各物理行は `type` を持つ JSON object。会話は `user` / `assistant` の `message.content` の string または text block 配列で、`uuid`、`sessionId`、`timestamp` などの record field を持つ。text と tool result/use は同じ record に共存できる。

直接親を示すには、同一物理親ファイルの Agent/Task `tool_use.id`、対応する `tool_result.tool_use_id`、record-level `toolUseResult.agentId` と子の agent ID の一致が必要。`fork-context-ref` は未取得の文脈への参照であり、本文自体ではない。子の最初の prompt や `parentUuid` だけでは直接親を確定しない。

## Codex

rollout は日付階層の `rollout-*.jsonl`。行の top-level は `timestamp`、`type`、`payload` を持つ形がある。`session_meta.payload.id` の最初の有効値が本人 ID。`source.subagent.thread_spawn.parent_thread_id` は明示直接親を示す。後続の埋込 metadata は本人 ID を変更しない。

会話は `response_item` のうち `payload.type=message`、`role=user|assistant`。`payload.content` 内の `input_text` / `output_text` / `text` block が本文を示す。tool call、reasoning、event は本人の会話本文と異なる。`subagent_history_start_ordinal` がある rollout では、top-level `ordinal` が境界未満の本文は継承文脈、境界以降は本人。境界があるのに ordinal がない本文の帰属は不明。境界のない形式に類似文や最初の user から境界を作らない。

## Cursor Agent

projects root からの相対 path は `<project-slug>/agent-transcripts/<session-id>/<session-id>.jsonl`。directory と basename の一致、空でない session ID が必要。slug は repository metadata ではない。UUID version による制限や `private-tmp` の特別除外は設けない。

既知の `role=user|assistant` record の `message.content` 配列には `type=text` と tool block がある。top-level `type=turn_ended` は会話本文でない。観測した形式には独立した timestamp、cwd、repository、version、title、usage、parent がない。本文内の `<timestamp>`、`<user_query>`、`<cwd>` tag は metadata の出典ではない。この local format は公式契約ではなく、形式変更時に再確認する。
