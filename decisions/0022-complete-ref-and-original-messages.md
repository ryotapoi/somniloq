---
status: current
---

# ADR 0022: 完全 REF から本人原文と関係を参照する

## 決定

入力・source・本人 identity を含む完全 REF を機械利用の参照単位とし、裸の session ID や短縮 REF による暗黙の選択を行わない。`search --session REF` と `show --descendants` は指定本人と直接親が確定した子孫に限る。root 所属だけが分かる子はまとまり一覧に残すが、確定した直接子として扱わない。

本人の user/assistant 原文 record に物理順の発言番号を付け、text block・元の空白・元の日時を保持する。`show` の発言フィルタと範囲はこの番号を共有し、以前の turn 番号、outline、表示除外、Markdown の独立入口は持たない。日時の保存値を日境界の設定で書き換えず、日付条件を query 時に適用する。まとまり一覧の開始・最終発言日時と、詳細・表示で使う個々の実発言日時を区別する。

外部利用者が同じ結果を再利用できるよう、REF、JSON envelope、TSV 列と escape を [CLI 公開形式](../docs/cli-contract.md) に定める。検索の `total` と項目は同じ read snapshot から返す。

## 理由

source と入力をまたぐ裸 ID は曖昧であり、曖昧さを自動解決すると別会話の原文を取得し得る。子からの探索を確定子孫へ限ると、兄弟や未確定の親を通じた予期しない範囲拡大を避けられる。表示用 turn や加工済み時刻を基準にすると、検索結果から物理的な本人原文へ戻れない。公開形式と同一 snapshot は、CLI を他の道具から利用するときの参照とページの一貫性に必要である。
