# v0.14.0 元ログの代表 fixture

すべて合成値であり、実ログの本文・ID・path をコピーしていない。v0.14.0 の受け入れ入力として作成し、現在は parser の回帰検証に使う。実ログの観測方法・範囲・限界は [時点付きの調査記録](../../../../cache/2026-10-05-log-evidence.md) に置く。合成入力の期待値はこの README と `expected.json`、現在の処理は parser と回帰テストで確認する。

`expected.json` は各ファイルの identity 配列、直接親、root 所属、本人発言の物理行・1始まり番号・本文・日時を示す。`kind` は `observed_structure`（観測した構造を合成値へ置換）と `designed_boundary`（意図的な境界）を区別する。mixed の値は観測構造と意図的な不一致を組み合わせた例である。`null` は未知であり、日時の補完・親の不存在確定ではない。source はディレクトリ名、入力は `input-a` / `input-b` の別 root として扱う。

| 対象 | 入力と期待値 |
| --- | --- |
| Codex 本人／継承 | `child.jsonl` の最初の metadata は child、2行目は埋込 root。3〜4行目は context、5行目 assistant が本人1番。`grandchild.jsonl` は child の子 |
| ordinal 無／境界無 | `no-boundary.jsonl` は ordinal も境界もなく全本文を本人として保持。`ordinal-no-boundary.jsonl` は ordinal だけあり同様。親本文との推測比較で除外しない |
| 所属不明 | `missing-ordinal.jsonl` は明示境界があるのに本文の ordinal がない意図的異常。本文は未確定として保持し、移行置換成功にしない |
| 同本人の複数 rollout | `child-extra.jsonl` と `child.jsonl` の辞書順で統合。payload ID と全文が一致する c1 のみ重複除外。`merged_order` はファイル単位の番号と混同しない |
| 入力境界 | `codex/input-a/root.jsonl` と `input-b/root.jsonl` は同名 ID で本文が違う。別入力なので別会話、別 REF、別置換対象 |
| 旧行の出自／誤帰属 | `legacy-rows.json` の UUID は `codex:` + SHA256の64桁小文字hex。子ファイルの行3〜6に一致する UUID が旧 root 会話に保存された例。行3〜4は継承、5〜6は child 本人。UUID だけで旧 root 全体を削除しない |
| Claude 厳密な子孫 | root の `call-child` と対応 result の構造化 `agentId=child`、child の `call-grandchild` と対応 result の `agentId=grandchild` を同一物理親内で照合。prompt は補助検証で、親推測には使わない |
| Claude 未確定 | root 5行目は `tool_use_id=other-call` で Agent call と不一致。`unresolved` 子は path から root 所属だけ分かり、直接親は未知。`fork-context-ref` の本文を仮造しない。`agent-agent-mismatch` はcall/result IDが一致しても結果agentIdが別で親未知 |
| Cursor 未知 | slug から project を復元せず、timestamp・parent は未知。1行目 text block は空行結合、tool-only/turn_ended は発言番号を消費せず4行目が2番 |
| 差分再開 | `resume/before.txt` の未完了2行目は未解析診断、再開位置は2行目先頭。`append.txt` を末尾に追記すると2行目が正常な assistant `After` となる |

## 処理順と再処理の期待結果

- 子・孫だけを先に import: Codex child/grandchild の直接親 ID は保持するが親本文を作らない。Claude grandchild は root 所属を保持し、child 内 call/result が未取得なら直接親は未知。親ファイル後着で照合できた時点で関係を解決する。REF と本人発言番号を変えない。
- 同入力・同ファイルを再 import: 本人本文・context・関係の重複を増やさない。親後着で本文のない疑似親を確定済み会話として残さない。
- 差分再開: `before.txt` では user `Before` が1番。追記後は assistant `After` が2番、行1の identity は不変。さらに全文再処理しても2発言のまま、同じ番号と未知日時を維持する。
- 入力を混ぜる: `input-b/root` は `input-a/child` の親を満たさない。同名 session ID・同名 payload ID の重複除外は入力をまたがない。

各 JSONL は最後の LF を含む。`resume/before.txt` は未完了tail、他のresume/*.txtは追記する断片なのでJSONLと別拡張子にしている。

## 複数 rollout の前方追記

`child-extra.jsonl` と `child.jsonl` の初回統合列は extra:2、extra:3、child:6（番号1、2、3）。`resume/forward-rollout-append.txt` の一行を既存 child-extra.jsonl の末尾へ追記すると、物理行4の新しいc4がcanonical列の3番となり、既存child:6は4番へ移る。前方rolloutへの追記なので本人全体を再構築・再採番する。番号を取り込み時刻の順に固定しない。

`expected.json` の `forward_rollout_append` が追記前後の出自列と番号を示す。同じ最終二ファイルから、通常差分import、全文再処理、migrate置換はすべて同じ4発言・同じ順序/番号を返す。これは意図的境界例であり実ログでの観測事実ではない。元のquery成功profileは追記前のファイル集合を使う。末尾だけに新本文が追加される単一rolloutの `resume/before.txt`→`append.txt` 例では既存番号は変わらない。
