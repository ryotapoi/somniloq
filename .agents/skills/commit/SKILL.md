---
name: commit
description: Use when the current repository changes are ready to be committed. Creates an English Conventional Commit after checking status, secrets risk, related docs, and backlog updates.
---

# Commit

## ICAR

- **Intent**: レビューと検証を通過した変更を、Conventional Commits 形式で記録する。
- **Constraints**:
  - コミットメッセージは英語。
  - summary は小文字始まり・70 文字以内・末尾ピリオドなし。
  - `.env`、credentials、不要な生成物はコミットしない。
  - `--no-verify` は使わない。
- **Acceptance**:
  - commit が作成されている。
  - `git status` で残差分が意図したものだけである。
- **Relevant**:
  - 変更差分
  - 検証結果
  - review 結果
  - `backlog/backlog.md`
  - `decisions/`
  - `references/knowledge.md`

## Checks

- 技術選定、データモデル、アーキテクチャ、将来制約になる判断は `decisions/` を検討する。
- 技術的なハマりどころは `references/knowledge.md` に残す。
- 完了した backlog 項目があれば `backlog/backlog.md` を更新する。
- `git status` と `git diff`（staged + unstaged）で変更内容を把握する。
- `git log --oneline -5` で直近のコミットスタイルを確認する。

## Message

```text
<type>: <summary>

<body if useful>

Co-Authored-By: Codex <noreply@openai.com>
```

Allowed types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `perf`.

## ADR Criteria

以下に該当する判断があった場合のみ ADR を作成する。

- 技術選定（ライブラリ、フレームワーク、プロトコル等）
- データモデルやスキーマの設計
- アーキテクチャパターンの選択
- 複数の選択肢から意図的に一つを選んだ判断
- 将来の実装に制約を与える判断
