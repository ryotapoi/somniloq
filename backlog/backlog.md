# Backlog

## 運用ルール

- Backlog には、今後着手でき、完了を判定できる作業を `- [ ]` 形式のタスクとして置く。
- 対応要否を決める調査もタスクにできる。観察事実だけを残さず、判断したい問いと、判断をもって完了とすることを明示する。
- 粗いメモで起票してよいが、着手前に目的と受け入れ条件を読める粒度へ詰める。
- バージョン番号を付ける場合は SemVer に従う。
- タスクは `###` 見出しでグループに分ける。バージョンが確定したグループだけ、`### v0.11.0 名前` のように見出しへ番号を付ける。
- グループが大きい場合は、`####` 見出しでさらに分割してよい。

## タスク

### v0.10.0 — Cursor Agent ログ対応

調査済みの前提: Cursor Agent `2026.09.10-fd3934a` を観測し、607 files / 9259 records を確認した。観測 path `~/.cursor/projects/<project-slug>/agent-transcripts/<session-id>/<session-id>.jsonl` は全件一致し、session directory と file name も一致する。record は `role=user|assistant` と `message.content` の text / tool_use、終了を示す top-level `type=turn_ended` からなり、独立した timestamp、cwd、repository、version、title、usage、parent 情報はない。controlled follow-up では既存 prefix を保った末尾追記を確認した。project slug は全件を信頼して可逆復元できない。

#### Cursor Agent ログ契約を fixture と仕様へ固定する

- [x] 機密本文を除いた代表 fixture と `docs/specs/jsonl-schema.md` に観測事実を記録し、理由が将来制約になる場合は ADR を追加する。
  - 受理対象 path と session 識別を固定する。user/assistant の text は順序を保って扱い、tool_use、turn_ended、未知の正常 record は会話本文にしない。壊れた JSON と既知 role の不正 content は既存の unparsed 契約で扱う。
  - 同じ入力を再処理しても session/message は重複せず、順序は安定する。ログにない metadata を事実として捏造しない。fixture と記録に機密本文を残さない。
  - `<timestamp>` / `<user_query>` / `<cwd>` の内部 tag を metadata 化または除去するか、private-tmp を含めるか、未知 metadata を内部でどう表すかは backlog で固定しない。実装時に調査事実と既存契約から Minimal Change を決める。これは追加のログ形式調査ではなく設計判断である。

#### Cursor Agent を既存 import に統合する

- [x] ユーザー向け `--source cursor-agent` と default all で Cursor Agent を取り込めるようにする。
  - source 単独と all で保存でき、root 不存在は既存の未使用 source として扱う。malformed を含んでも既存の import error / unparsed 契約に従う。
  - 追記後の再 import は新規会話だけを追加し、既存データを重複させない。縮小と `--full` は既存契約を維持し、Claude Code / Codex import を回帰させない。

#### 横断参照と利用者向け文書を同期する

- [x] sessions / projects / search / show / outline で Cursor を他 source と同じ DB から扱えるようにする。
  - source を区別でき、同じ session ID の ambiguity は既存契約に従う。欠落 metadata を誤表示または誤 filter せず、既存 source を回帰させない。
  - `docs/rules/mission.md`、`docs/rules/scope.md`、README 英日、CLI help など、実際に影響する正本または派生文書だけを同期する。`docs/decisions/` と `llm-wiki/` は必要な場合に更新する。検証は `docs/rules/verification.md` に従う。

#### example skill を Cursor Agent 対応へ更新する

- [x] `examples/skills/somniloq/SKILL.md` の front matter description と概要文に Cursor Agent を追加する。
  - Claude Code / Codex / Cursor Agent の履歴を対象と明記し、既存の `somniloq import` から sessions / search / outline / show へ進む導線が現行 CLI 挙動と矛盾しない。

#### 既知の制約

- local format は公式契約ではなく version 依存である。形式変化が観測された時に再調査する。
- timestamp / repository metadata、usage、親子、tool 履歴はログから完全復元できない。v0.10.0 でどこまで扱うかは goal ごとの Minimal Change で判断する。ただし、捏造しない。
