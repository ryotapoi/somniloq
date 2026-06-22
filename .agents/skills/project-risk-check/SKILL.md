---
name: project-risk-check
description: Use for somniloq-specific plan or implementation checks when changes touch SQLite schema or migrations, backfill or DELETE behavior, SQL semantics, JSONL import boundaries, CLI output contracts, or cmd/internal module boundaries.
---

# somniloq Risk Check

## ICAR

- **Intent**: somniloq 固有の mission・アーキテクチャ制約・既知の落とし穴に照らして、計画または実装のリスクを確認する。
- **Constraints**:
  - 汎用レビューではなく、somniloq 固有の実害に絞る。一般的なコード品質は `change-review`、構造劣化は `thermo-nuclear-code-quality-review` 側で見る。
  - 仕様・CLI 挙動・データ保持・削除方針の判断が必要なら、実装判断として決めずユーザー確認に回す。
  - 具体的な過去知見はソースコメントや `llm-wiki/` の地図を参照し、skill 本体には増やしすぎない。
  - plan / 実装どちらのレビューでも使える。対象は plan ファイル、または未コミット差分 / commit range。
- **Acceptance**:
  - `LGTM` またはリスク一覧がある。
  - リスクには影響、根拠、推奨対応がある。
  - 必要な場合、更新すべき `docs/specs/`, `backlog/backlog.md`, `docs/decisions/`、および知見の記録先（ソースコメント / `llm-wiki/`）が明確。
- **Relevant**:
  - ユーザー依頼、plan、または変更差分（未コミット / commit range）
  - `docs/rules/mission.md`
  - `docs/rules/scope.md`
  - `docs/rules/architecture.md`
  - `docs/rules/constraints.md`
  - `docs/rules/information-management.md`
  - 関連する `docs/specs/`
  - `docs/specs/jsonl-schema.md`
  - `llm-wiki/`（作業地図）

## Checkpoints

### Mission / Scope

- 「Claude Code / Codex のセッションログを SQLite に保存・検索する CLI」という mission から外れていないか。

### Architecture / 依存方向

- `cmd/somniloq -> internal/core` の依存方向を守っているか。`internal/core` が `cmd/somniloq` に依存していないか。
- `cmd/somniloq` は CLI 入出力・フラグ解析・表示整形に留まり、DB 操作や JSONL パースを持ち込んでいないか。
- `internal/core` は JSONL パース・DB スキーマ管理・インポート・クエリに留まり、CLI 入出力や `os.Exit` を持ち込んでいないか。
- `cmd/somniloq` と `internal/core` 間で共有する概念が、依存方向に沿って配置されているか。
- 構造変更と新しいビジネスロジックが 1 つの plan / diff に混在していないか。

### DB / SQL semantics

- JSONL 由来のデータは必ず SQL プレースホルダ経由で渡しているか。文字列連結で SQL を組み立てていないか。
- `modernc.org/sqlite` の `ON CONFLICT DO NOTHING` 時に、`LastInsertId()` を `RowsAffected()` 確認なしで使っていないか。
- `:memory:` 接続、`PRAGMA table_info` ベースの migration 判定など、SQLite / modernc.org/sqlite の既知の罠を踏んでいないか。
- 文字列カラムの `MAX` を「最新」や「代表」として扱っていないか。
- GROUP BY キーと表示キーの短縮・変換で情報が縮退し、同名行が出現していないか。

### Backfill / migration / DELETE

- SQLite schema / migration / `backfill` / DELETE の変更で、再実行性・既存 DB・確認プロンプト・非対話環境（`--yes` 経路）が検証されているか。
- フィルタ・スキップ・バリデーションの削除や変更で、既存 DB に保存済みのデータへの影響を見落としていないか。

### JSONL import boundaries

- Claude Code / Codex JSONL の形式差、未知フィールド、メタのみセッション（`custom-title` のみ等）、空 text、差分取り込みキーを壊していないか。

### CLI stable interface

- stdout/stderr の使い分け、TSV/Markdown 出力、exit code、usage/help、確認プロンプトが既存仕様と同期しているか。
- `--project`, `--since`, `--until`, `--summary`, `--short` など検索・表示オプションの意味を意図せず変えていないか。
- 新しいフラグやモードが usage 定数・ヘルプ文字列に反映されているか。
- 出力フォーマット変更がスクリプト連携や TSV パース等に影響する場合、破壊的変更として明示されているか。

### 実装の正確性

- 複数ファイル・レコードを処理するループで、タイムスタンプをループ外で 1 回だけ取得していないか。

### テスト / ドキュメント同期

- テストが実装の現状追認ではなく、意図した仕様を検証しているか。
- 既存の類似機能にあるテスト観点が、新機能のテストにも含まれているか。
- CLI 表面仕様の変更に伴い、README、`docs/rules/scope.md`、`docs/specs/jsonl-schema.md` 等の更新が含まれているか。
- plan 内の設計判断、型・値・前提条件が矛盾していないか。

上記に該当しないが somniloq 固有の設計判断に関わる問題も自由に指摘してよい。

## Output

- 日本語。指摘には 🔴 MUST / 🟡 SHOULD / 🔵 NIT を付け、該当箇所を引用する。
- somniloq 固有の問題がなければ「somniloq 固有の指摘なし（LGTM）」。
