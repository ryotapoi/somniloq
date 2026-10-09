---
status: current
---

# ADR 0021: 入力 identity と明示設定

## 決定

Claude Code、Codex、Cursor Agent を共通の DB と `import` で扱う。入力は source と実体 root の組で区別し、同じ session ID でも別入力の本人を統合しない。入力の表示名は identity に含めない。同じ実体 root の重複設定は一入力として走査する。

設定は DB と入力を TOML で明示する。旧 JSON 設定を自動探索・変換せず、通常操作に DB path の一時 override を設けない。設定がない状態では DB を暗黙に作らない。`import` の source と input の選択は交差させ、`--full` は選択した入力だけを再構築する。

project alias は検索時に完全一致で展開し、保存した repository 値を書き換えない。別 group と曖昧に重なる alias 設定は拒否する。

## 理由

異なる source や root に同名の ID が現れ、表示名も変更できる。保存や REF の identity を ID・表示名だけに結びつけると、別の履歴が潰れたり、改名で参照が変わったりする。設定と選択範囲を明示すると、通常取り込みや `--full` が別入力の履歴を消すことを防げる。alias を保存時に反映すると、設定変更が過去の保存値の意味を変えてしまう。

専用 `backfill` は提供しない。既知の旧形式の履歴保持だけを [ADR 0024](0024-owner-history-migration.md) の限定 `migrate` で扱う。
