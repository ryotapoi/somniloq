# Implement Workflow

## ICAR

- **Intent**: 承認済み plan、または plan を省略できる軽微な変更の明確な要求を、既存設計と情報源に整合する形で実装する。
- **Constraints**:
  - 既存の局所パターンに従う。変える場合は理由を説明できるようにする。
  - 型定義・API・依存方向は実物で確認する。
  - 振る舞い変更があるなら、必要に応じて `specs/`、`rules/scope.md`、README、テストを同期する。
  - 実装中に見つかった別タスクは、今やる理由がなければ `backlog/backlog.md` に逃がす。
  - ループ内で時刻を扱う場合は各反復で取得する（ループ外で 1 回だけ取得しない）。
- **Acceptance**:
  - 要求された振る舞いが実装されている。
  - 必要な `specs/` / tests / `backlog/backlog.md` / README の同期が済んでいる。
  - 余計なスコープ拡張がない。
- **Relevant**:
  - 承認済み plan、または Small 変更の明確な要求
  - 関連する `rules/`, `specs/`, `references/knowledge.md`, `references/jsonl-schema.md`
  - 変更対象と周辺コード

## Flow ICAR

### Code Change

- **Intent**: 要求された振る舞いを最小十分な差分で実装する。
- **Constraints**:
  - Go の標準的なパターンと既存コードの局所スタイルに合わせる。
  - `internal/core` に CLI 入出力や `os.Exit` を持ち込まない。
  - `cmd/somniloq` に DB 操作や JSONL パースを持ち込まない。
  - SQLite/JSONL/CLI 仕様に触れる変更はテストを先に置くか、少なくとも仕様を検証するテストを追加する。
- **Acceptance**: plan と実装上の事実が食い違っていない。
- **Relevant**: 変更対象コード、関連テスト、関連 specs / rules。

### Documentation Sync

- **Intent**: 実装で変わった仕様・知見・未着手作業を正しい情報源に反映する。
- **Constraints**:
  - 完了した backlog 項目があれば `backlog/backlog.md` を更新する。
  - 技術的知見は `references/knowledge.md` に残す。
  - JSONL 形式の参照情報は `references/jsonl-schema.md` に残す。
  - 後から制約になる判断は `decisions/` に残す。
- **Acceptance**: 実装差分と情報源が矛盾していない。
- **Relevant**: `specs/`, `rules/`, `backlog/backlog.md`, `decisions/`, `references/knowledge.md`。

## Go Tooling

- ビルド: `go build -o bin/somniloq ./cmd/somniloq`
- テスト: `go test ./...`
- 静的チェック: `go vet ./...`
- フォーマット: `gofmt` / `goimports`

## Stop Conditions

- plan と実装上の事実が食い違う。
- 実装中に仕様判断が必要になった。
- リファクタなしでは変更が不自然または危険になる。
