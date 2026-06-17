# Implement Workflow

## ICAR

- **Intent**: 承認済み plan、または plan を省略できる軽微な変更の明確な要求を、既存設計と情報源に整合する形で実装する。
- **Constraints**:
  - 既存の局所パターンに従う。変える場合は理由を説明できるようにする。
  - 型定義・API・依存方向は実物で確認する。
  - TDD でやる場合は `tdd` スキルに従う。Normal / High-risk の振る舞い変更は基本 TDD とし、Small は省略可。
  - 振る舞い変更があるなら、必要に応じて `docs/specs/`、`docs/rules/scope.md`、README、テストを同期する。
  - 振る舞い変更や bug fix では、同じ commit に unit test / regression test を追加または更新する。テストできない場合は理由を明記する。
  - 実装中に見つかった別タスクは、今やる理由がなければ `backlog/backlog.md` に逃がす。
  - 構造の悪さが実装を歪める場合は、同じ変更で直すか、別リファクタ plan に切るかを判断する。
  - ループ内で時刻を扱う場合は各反復で取得する（ループ外で 1 回だけ取得しない）。
- **Acceptance**:
  - 要求された振る舞いが実装されている。
  - 必要な `docs/specs/` / tests / `backlog/backlog.md` / README / `llm-wiki/` の同期が済んでいる。
  - 余計なスコープ拡張がない。
- **Relevant**:
  - 承認済み plan、または Small 変更の明確な要求
  - 関連する `docs/rules/`, `docs/specs/`, `llm-wiki/`, `docs/specs/jsonl-schema.md`
  - 変更対象と周辺コード

## Flow ICAR

### Code Change

- **Intent**: 要求された振る舞いを最小十分な差分で実装する。
- **Constraints**:
  - Go の標準的なパターンと既存コードの局所スタイルに合わせる。
  - `internal/core` に CLI 入出力や `os.Exit` を持ち込まない。
  - `cmd/somniloq` に DB 操作や JSONL パースを持ち込まない。
  - SQLite/JSONL/CLI 仕様に触れる変更はテストを先に置くか、少なくとも仕様を検証するテストを追加する。
  - 振る舞い変更や bug fix では unit test / regression test を同じ commit に含め、含められない場合は理由を残す。
- **Acceptance**: plan と実装上の事実が食い違っていない。
- **Relevant**: 変更対象コード、関連テスト、関連 specs / rules。

### Documentation Sync

- **Intent**: 実装で変わった仕様・知見・未着手作業を正しい情報源に反映する。
- **Constraints**:
  - 完了した backlog 項目があれば `backlog/backlog.md` を更新する。
  - 特定ソースを編集するときだけ必要な罠は、そのソースのコメントに残す。横断的な挙動・設計理解は `llm-wiki/` に残す。
  - 今回の変更で `llm-wiki/` が古くなっていないか見て、同じ差分の中で追従する。追従更新は commit 待ちにせず、review で差分の一部として見る。
  - `llm-wiki/` の追従では、`docs/rules/information-management.md` の `regen` 区分に従う。索引・地図（例: `llm-wiki/command-map.md`, `regen: full`）は frontmatter の `sources:` を読み直し、古くなった節をソースから再生成する。概念・ガイド（例: `llm-wiki/import-pipeline.md`, `regen: compiled`）は読む順序・経路・注意点を再編纂する。外部知見（例: `llm-wiki/sqlite-driver-notes.md`, `regen: none`）は横断的なものだけ手で育て、特定ソースに紐づく罠はコードコメントへ寄せる。
  - `llm-wiki/` に単一の集約知見ファイルを作らない。仕様や判断を拘束し始めた情報は docs へ昇格する。
  - JSONL 形式の参照情報は `docs/specs/jsonl-schema.md` に残す。
  - 後から制約になる判断は `docs/decisions/` に残す。
- **Acceptance**: 実装差分と情報源が矛盾していない。
- **Relevant**: `docs/specs/`, `docs/rules/`, `backlog/backlog.md`, `docs/decisions/`, `llm-wiki/`。

## Go Tooling

- ビルド: `go build -o bin/somniloq ./cmd/somniloq`
- テスト: `go test ./...`
- 静的チェック: `go vet ./...`
- フォーマット: `gofmt` / `goimports`

## Stop Conditions

- plan と実装上の事実が食い違う。
- 実装中に仕様判断が必要になった。
- リファクタなしでは変更が不自然または危険になる。
