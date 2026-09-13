# Cursor Agent 合成 fixture

`cursor-agent.jsonl` は実ログをコピーしていない合成・匿名化済みの境界入力である。本文、path、ID、tool input は無害な架空値のみを使う。観測した record 形状を再現する行は 1、2、4 であり、それ以外は仕様の境界を検証するために追加した合成行である。

次の実装では、この fixture を Cursor projects root 配下の
`synthetic-project/agent-transcripts/session-sample/session-sample.jsonl` として扱う。これは受理例であり、session identity は Cursor source と `session-sample` である。

受理しない代表例は、`synthetic-project/session-sample.jsonl`（構造外）、
`synthetic-project/agent-transcripts//session-sample.jsonl`（空 directory）、
`synthetic-project/agent-transcripts/session-sample/other.jsonl`（basename 不一致）である。
`private-tmp/agent-transcripts/session-sample/session-sample.jsonl` は、特別扱いせず同じ規則で受理する。

| 物理行 | 入力の分類 | 期待する本文または結果 |
|---|---|---|
| 1 | 観測形状: user text | message: `Plan a harmless sample.` |
| 2 | 観測形状: assistant text + tool_use + text | 1 message。本文は `First answer paragraph.\n\nSecond answer paragraph.`。tool_use は保存しない。 |
| 3 | 境界入力: user tool_use のみ | 意図的無視。session は登録できるが message は作らない。 |
| 4 | 観測形状: turn_ended | 意図的無視。 |
| 5 | 境界入力: 未知 role の正常 record | 意図的無視。 |
| 6 | 境界入力: 既知 role の空 content | session は登録できるが message は作らない。 |
| 7 | 境界入力: 内部 tag を含む text | message: `Keep <timestamp>, <user_query>, and <cwd> as text.`。tag を除去・抽出しない。 |
| 8 | 境界入力: 空行 | 意図的無視。物理行番号には含める。 |
| 9 | 境界入力: 壊れた JSON | unparsed。部分本文を保存しない。 |
| 10 | 境界入力: 既知 role の不正 content envelope | unparsed。 |
| 11 | 境界入力: text が非文字列 | unparsed。部分本文を保存しない。 |

message identity は source、fixture path、物理行で決定的に導く。したがって、この入力を再処理しても lines 1、2、7 の message は重複せず、順序は 1、2、7 のまま安定する。空行・無視行・unparsed 行も line identity の番号をずらさない。

この fixture は次の adapter 実装の契約を確認するためのものだが、fixture 専用 parser や Cursor production package をここで追加しない。現行 CLI は Cursor Agent をまだ import しない。
