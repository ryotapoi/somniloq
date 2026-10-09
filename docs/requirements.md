# 要件と制約

somniloq は Claude Code、Codex、Cursor Agent の JSONL セッションログを SQLite に保存し、source・入力・セッションをまたいで CLI から検索・原文参照できるようにする。Daily Note 等から過去の作業と指定日の実発言へ戻れることを目的とする。

リアルタイム監視、ストリーミング、Web UI、GUI、LLM による要約は対象外とする。本文の類似性や近い時刻から、別の起動・直接親・欠けた履歴をつながない。互換性維持だけの shim、deprecated、fallback 分岐を増やさない。

## 保存する証拠

- source の物理的な出自、本人と直接親の証拠、root 所属を区別する。証拠のない入力 identity、日時、repository、親、継承本文は補完しない。元ログが失われた旧履歴は由来不明のまま保持する。
- 原文の user/assistant record、text block 境界、空白、元 timestamp、source の物理順を保持する。検索・表示の発言番号は本人原文列を基準とし、timestamp や SQLite rowid で付け直さない。
- 本人原文列・文脈・関係・対象 cursor を再構築するときは一つの transaction で確定し、失敗時に旧本文と cursor を部分更新しない。通常差分と full は同じ最終ファイル集合から同じ列・番号を得る。
- 通常 import の不完全な走査で安全性が証明できない本人は既存保存を守る。専用 migrate は既知の旧 snapshot だけを受理し、元ログや旧 DB を変更せず、正常解析した本人 ID と同名の Codex 旧履歴は本人本文0件も含めて置換する。別 ID の所属を証明できない旧行は消さず、履歴を失わせる推測置換をしない。
- 検索の items、total、関係は一つの read snapshot に基づく。日時不明の原文は日時条件に推測で一致させず、条件なしでは参照できる。

## SQL と公開境界

- JSONL 由来の値は SQL placeholder に bind し、文字列連結で SQL を構成しない。
- 任意の文字列列の `MAX()` を「最新」や「代表」の選択に使わない。固定幅に正規化された RFC3339 UTC timestamp だけは辞書順と時刻順が一致する。集約の GROUP BY key と表示値に別々の短縮・変換を適用しない。
- CLI の外部参照と出力形式は [CLI 公開形式](cli-contract.md)、source の入力形状は [JSONL 形式](jsonl-format.md) に定める。利用コマンドと例は README と help に置く。
