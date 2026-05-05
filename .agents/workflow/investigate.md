# Investigate Workflow

## ICAR

- **Intent**: 計画や実装に入る前に、必要な事実・不明点・判断材料を揃える。
- **Constraints**:
  - 何が分かれば plan / direct implement / stop に進めるかを先に定義する。
  - コードだけで分からない挙動は、実行・計測・公式資料確認・ユーザー確認に切り替える。
  - CLI 出力や期待挙動が曖昧なら、コードから推測を重ねずユーザーに確認する選択肢を持つ。
  - subagent は、複数ファイル横断・広域 grep・独立した仮説検証を並列化できる場合に使う。
  - 調査中の一時コードや一時データは、残す理由がなければ最終成果に含めない。
- **Acceptance**:
  - 判明した事実と残った不明点が説明できる。
  - 次に plan / direct implement / stop のどれに進むか判断できる。
  - 永続化が必要な知見・要求変更が適切な場所に記録されている。
- **Relevant**:
  - ユーザー依頼
  - `backlog/backlog.md` の該当項目
  - 関連する `rules/`, `specs/`, `decisions/`, `references/knowledge.md`, `references/jsonl-schema.md`
  - 既存コード、ログ、再現手順

## Use When

- 原因不明のバグ
- 仕様や期待挙動が曖昧
- JSONL 実例、SQLite 結果、CLI 出力、エラー時挙動の確認が必要
- 技術検証が必要
- 実機やユーザー環境に依存する観察が必要

## Recording

- 調査結果が将来も効くなら `references/knowledge.md` に残す。
- JSONL の形式やスキーマ差異の参照情報なら `references/jsonl-schema.md` に残す。
- 要求や粒度が変わるなら `backlog/backlog.md` に残す。
- 後から理由を問われる判断なら `decisions/` に残す。

## Stop Conditions

- ユーザーの観察・判断なしに確定できない挙動がある。
- 調査結果により、元の要求やスコープが変わった。
- 検証のための一時変更を残すか戻すか判断が必要。
