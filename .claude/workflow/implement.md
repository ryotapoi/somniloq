# Implement

## Intent

承認済み plan、または plan を省略できる軽微な変更の明確な要求を、既存設計と情報源に整合する形で実装する。

## Inputs

- 承認済み plan、または Small 変更（`default.md` の Intake 分類）の明確な要求
- 関連する `docs/rules/`, `docs/specs/`（あれば）, `llm-wiki/`
- 変更対象と周辺コード

## Decision Criteria

- 既存の局所パターンに従う。変える場合は理由を説明可能にする
- 型定義・API・依存方向は実物で確認（推測しない）
- TDD でやる場合は `tdd` スキルに従う（Normal / High-risk の振る舞い変更は基本 TDD。Small は省略可）
- 振る舞い変更や bug fix では、同じ commit に unit test / regression test を追加または更新する。テストできない場合は理由を明記する
- 振る舞いが変わるなら `docs/specs/`（あれば）の該当箇所を同期する
- backlog に積んでいた項目を実装完了したら `backlog/backlog.md` の該当行を `[x]` 等で更新する
- 今回の変更で `llm-wiki/` が古くなっていないか見て、同じ差分の中で追従する。追従更新は commit 待ちにせず、review で差分の一部として見る
- `llm-wiki/` の追従では、`docs/rules/information-management.md` の `regen` 区分に従う。索引・地図（例: `llm-wiki/command-map.md`, `regen: full`）は frontmatter の `sources:` を読み直し、古くなった節をソースから再生成する。概念・ガイド（例: `llm-wiki/import-pipeline.md`, `regen: compiled`）は読む順序・経路・注意点を再編纂する。外部知見（例: `llm-wiki/sqlite-driver-notes.md`, `regen: none`）は横断的なものだけ手で育て、特定ソースに紐づく罠はコードコメントへ寄せる
- `llm-wiki/` に単一の集約知見ファイルを作らない。仕様や判断を拘束し始めた情報は docs へ昇格する
- 実装中に見つかった別タスクは、今やる理由がなければ `backlog/backlog.md` に逃がす
- 構造の悪さが実装を歪める場合は、同じ変更で直すか、別リファクタ plan に切るかを判断する
- ループ内で時刻を扱う場合は各反復で取得（ループ外で 1 回だけ取得しない）

## Go Tooling

- ビルド: `go build ./...`
- テスト: `go test ./...`
- 静的チェック: `go vet ./...`
- フォーマット: `gofmt` / `goimports` は PostToolUse hook（`.claude/hooks/go-format.sh`）で自動実行されるため手動不要

## Acceptance

- 要求された振る舞いが実装されている
- 必要な `docs/specs/`（あれば） / tests / `backlog/backlog.md` / `llm-wiki/` の同期が済んでいる
- 余計なスコープ拡張がない

## Stop Conditions

- plan と実装上の事実が食い違う
- 実装中に仕様判断が必要になった
- リファクタなしでは変更が不自然または危険になる
