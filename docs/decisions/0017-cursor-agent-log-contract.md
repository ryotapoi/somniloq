# ADR 0017: Cursor Agent の入力契約を観測事実と分離して固定する

## Status

Accepted（2026-09-14 決定）

## Context

Cursor Agent `2026.09.10-fd3934a` のローカルログ 607 files / 9259 records を調べたところ、
`~/.cursor/projects/<project-slug>/agent-transcripts/<session-id>/<session-id>.jsonl` の path 構造、
`user` / `assistant` の content block、`tool_use`、`turn_ended`、および prefix を保つ末尾追記を確認した。
一方、timestamp、cwd、repository、version、title、usage、parent はログから得られず、slug の可逆性も確認できなかった。

次の Change で adapter を実装できるよう、実ログの機密本文を repository へ保存せず、観測済みの形状と somniloq が採用する振る舞いを分けて固定する必要がある。現行の CLI / DB は Cursor Agent をまだ扱わない。

## Considered Options

- **A: 観測された path だけを許可し、content と metadata は実装時に決める**: 早く着手できるが、text の順序、unparsed 境界、欠落 metadata、再処理の安定性を後から解釈し直すことになる。
- **B: 任意の `.jsonl` を候補にし、slug や本文 tag から不足 metadata を補完する**: source の形式変更には広く見えるが、別の JSONL を誤受理し、観測されていない事実を捏造する。
- **C: path と record の最小契約、意図的無視 / unparsed の境界、欠落 metadata の表現を仕様と匿名 fixture に固定する**: adapter は追加で必要になるが、未観測情報を補わずに入力境界を判定できる。

## Decision

Option C を採用する。現行結論は `docs/specs/jsonl-schema.md` の Cursor Agent 節に置き、
`internal/ingest/testdata/cursor-agent/` に合成・匿名化済み fixture と行単位の期待結果を置く。

受理する path は Cursor projects root から
`<project-slug>/agent-transcripts/<session-id>/<session-id>.jsonl` に限定する。session directory と basename の一致、および空でない session ID を要求するが、観測根拠のない UUID version 制限は加えない。`private-tmp` は観測事実として扱わず、特別除外の根拠がないため一般規則を適用する。

既知 role の text block を配列順に空行で結合し、物理行順を維持する。tool、turn、未知正常 record、空行は意図的無視し、壊れた JSON と既知 role の壊れた envelope / 非文字列 text は unparsed とする。message identity は source、path、物理行から決定的に導き、再処理で重複も順序変化も起こさない。timestamp を合成して順序を作らない。

欠落した metadata は string を空値、parent を nil として既存の正規化・永続化規則へ渡す。mtime、import 時刻、slug、本文内 tag を metadata の出典にしない。`<timestamp>`、`<user_query>`、`<cwd>` を含む text は本文のままとする。

この ADR は Cursor adapter、source enum、CLI、SQL schema、migration、表示仕様を導入しない。それらは次の Change の責務である。

## Consequences

- 次の adapter 実装は、受理 / 拒否 path、session identity、text 抽出、無視 / unparsed、物理行 identity を仕様と fixture から直接照合できる。
- 合成 fixture は実ログの観測結果を証明するものではなく、契約の境界を検証するための例である。
- Cursor の外部形式は version 依存のため、実装時または将来の形式変化時に実環境での互換性確認が別途必要である。
- 現行 CLI が Cursor Agent を import できるという主張はしない。
