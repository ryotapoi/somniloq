---
name: somniloq-risk-check
description: somniloq 固有の plan / 実装チェック。SQLite スキーマ・マイグレーション、backfill・DELETE を伴う処理、SQL の意味的変更、JSONL 取り込みの境界、CLI 出力仕様、cmd/internal のモジュール境界に触れる変更で使う。汎用レビューではなく somniloq 固有の実害に絞って確認する。
---

# somniloq Risk Check

## Intent

somniloq 固有の mission・アーキテクチャ制約・既知の落とし穴に照らして、計画または実装のリスクを確認する。

このチェックは main で直接回さず、fork 構造で実行する。main は監督 subagent を起動するだけで、実際の Checkpoints 照合は監督配下の subagent が並列に行う。手順は下記「実行構造」を参照。Checkpoints（チェック観点そのもの）は変更しない。

## 実行構造（fork）

main は監督を起動するだけで、Checkpoints 照合は監督配下の subagent に分散する。main コンテキストを汚さず、最終判断（修正・同期）を main に残すための fork。

1. **監督起動（main → Opus）**: main は `Agent` ツールで risk-check 監督を 1 体起動する。`model` は `opus` を明示する（main の context 隔離が目的で、最終採否・修正・同期判断は main に残るため。親モデル継承による高コストも避ける）。prompt には次を渡す:
   - 対象（plan ファイルのパス / 未コミット差分 / commit range のいずれか）
   - 参照すべきパス: `docs/rules/mission.md`, `docs/rules/scope.md`, `docs/rules/architecture.md`, `docs/rules/constraints.md`, `docs/rules/information-management.md`, `llm-wiki/`, `docs/specs/jsonl-schema.md`
   - worktree 作業中なら CLAUDE.md の定型（作業ディレクトリのフルパス明記）
2. **観点クラスタへの分割（監督 → sonnet ×2〜5）**: 監督は下記 Checkpoints を観点クラスタに分け、subagent に振り分けて並列起動する。各 subagent は `model: sonnet` を明示する。クラスタ例:
   - (a) ミッション・スコープ（Checkpoint 1）
   - (b) モジュール配置・構造 / 依存方向（Checkpoint 2〜4）
   - (c) ドメイン semantics（DB・SQL の安全性、backfill・migration・DELETE、JSONL 取り込みの境界、CLI の安定インターフェース、実装の正確性: Checkpoint 5〜17）
   - (d) 既知の落とし穴 + llm-wiki 照合、テスト検証の網羅性、派生ドキュメント・plan 内整合（Checkpoint 6〜7・18〜21 と `llm-wiki/` 突き合わせ）
   - 対象が小さい場合は観点をまとめて体数を減らしてよい（最小 2 体）。クラスタの切り方は対象に合わせて調整してよいが、全 Checkpoint をいずれかのクラスタが必ずカバーする。
3. **各 subagent への指示**: 各 subagent には「ファイルパス・行番号つきの事実と該当 Checkpoint 番号のみ返す。推測・提案・『推奨対応』セクションは含めない」を必ず指示する（判断は監督と main に残す）。
4. **統合（監督 → main）**: 監督は各 subagent の結果を dedup し、重大度を付けて一覧に統合して返す。重大度は 🔴（仕様逸脱・データ破壊級）/ 🟡（要対応の懸念）/ 🔵（軽微・確認推奨）。監督は修正を一切行わない。固有の指摘がなければ「固有の指摘なし（LGTM）」を返す。
5. **修正・同期（main）**: 監督が返した一覧をもとに、修正と、`docs/specs/`（あれば）/ `backlog/backlog.md` / `docs/decisions/` / `llm-wiki/` への同期判断は main 側で行う。

## Constraints

- 汎用レビューではなく、somniloq 固有の実害に絞る。一般的なコード品質・構造劣化は汎用レビュー側で見る（Claude では `/code-review` / `thermo-nuclear-code-quality-review`）。
- 仕様・CLI 挙動・データ保持・削除方針の判断が必要なら、実装判断として決めずユーザー確認に回す。
- 具体的な過去知見は `llm-wiki/` を参照し、skill 本体には増やしすぎない。
- plan / 実装どちらのレビューでも使える。対象は plan ファイル、または未コミット差分 / commit range。
- Checkpoints と対象を照合する際、必要に応じて `docs/rules/` と `llm-wiki/` を Read で読む。

## Acceptance

- `LGTM` またはリスク一覧がある。
- リスクには影響、根拠、推奨対応がある。
- 必要な場合、更新すべき `docs/rules/`, `backlog/backlog.md`, `docs/decisions/`, `llm-wiki/`, `docs/specs/jsonl-schema.md`, `docs/specs/`（あれば）が明確。

## Relevant

- ユーザー依頼、plan、または変更差分（未コミット / commit range）
- `docs/rules/mission.md`
- `docs/rules/scope.md`
- `docs/rules/architecture.md`
- `docs/rules/constraints.md`
- `docs/rules/information-management.md`
- `llm-wiki/`
- `docs/specs/jsonl-schema.md`

## Checkpoints

### ミッション・スコープ

1. **mission からの逸脱**: 「Claude Code / Codex のセッションログを SQLite に保存・検索する CLI」から外れていないか（`docs/rules/mission.md`）。

### モジュール配置・構造

2. **依存方向 `cmd/somniloq → internal/core` の遵守**: 新しい import がこの方向に従っているか。`internal/core` が `cmd/somniloq` に依存していないか（`docs/rules/architecture.md` 参照）。
3. **共通化は依存方向に沿って配置する**: `cmd/somniloq` と `internal/core` 間で共有するコードは `internal/core` に置く。`cmd/somniloq` のローカルなヘルパーが本来 `internal/core` に属する概念を扱っていないか。
4. **リファクタリングと機能実装を混ぜない**: diff / plan のステップに構造変更と新しいビジネスロジックが混在していないか。必要なら先行リファクタとして分離する。

### DB・SQL の安全性

5. **SQL プレースホルダの使用**: JSONL 由来のデータは必ず `?` プレースホルダ経由で SQL に渡す。文字列結合で SQL を組み立てない。
6. **modernc.org/sqlite の LastInsertId の罠**: `ON CONFLICT DO NOTHING` 時、`LastInsertId()` は前回挿入の rowid を返す。`RowsAffected()` を先にチェックする。
7. **modernc.org/sqlite のその他の罠**: `:memory:` 接続、`PRAGMA table_info` ベースの migration 判定など。詳細は `llm-wiki/`。
8. **SQL 集約関数の意味的正しさ**: 文字列カラムの MAX は辞書順最大値であり、「最新」や「代表」とは限らない。
9. **集計キーと表示キーの整合**: GROUP BY キーと表示の短縮・変換で情報が縮退し、同名行が出現しないか。集計キーも合わせて寄せる必要がないか。

### backfill・migration・DELETE

10. **再実行性と既存 DB への影響**: SQLite schema / migration / `backfill` / DELETE の変更で、再実行性・既存 DB・確認プロンプト・非対話環境（`--yes` 経路）の扱いが検証されているか。
11. **既存データへの影響考慮**: フィルタ・スキップ・バリデーションの削除や変更時、既存 DB に保存済みのデータへの影響が検討されているか。「新規データだけ正しくなる」で済まない場合がある。

### JSONL 取り込みの境界

12. **形式差・境界ケースを壊さない**: Claude Code / Codex JSONL の形式差、未知フィールド、メタのみセッション（`custom-title` のみ等）、空 text、差分取り込みキーを壊していないか（`docs/specs/jsonl-schema.md`）。

### CLI の安定インターフェース

13. **CLI 出力仕様の同期**: stdout/stderr の使い分け、TSV/Markdown 出力、exit code、usage/help、確認プロンプトが既存仕様と同期しているか。
14. **検索・表示オプションの意味維持**: `--project`, `--since`, `--until`, `--summary`, `--short` など検索・表示オプションの意味を意図せず変えていないか。
15. **Usage / ヘルプ文字列の網羅性**: 新しいフラグやモードが usage 定数・ヘルプ文字列に反映されているか。既存の usage に欠落があっても、今回追加したフラグは含める。
16. **互換性への影響の明示**: 出力フォーマット変更が既存の利用パターン（スクリプト連携、TSV パース等）に影響する場合、破壊的変更として明示されているか。

### 実装の正確性

17. **ループ内の時刻取得**: 複数ファイル・レコードを処理するループで、タイムスタンプをループ外で 1 回だけ取得していないか。処理時間が長い場合、各反復で取得すべき。

### テスト検証の網羅性

18. **テストが意図した仕様を検証しているか**: 「現在の実装の動作」の追認になっていないか。実装のバグをテストが正として固定化していないか。
19. **既存テストとの対称性**: 既存の類似機能にあるテスト観点（例: ミリ秒境界テスト）が、新機能のテスト（計画）にも含まれているか。片方だけテストがあると退行を検出できない。

### 派生ドキュメント・plan 内整合

20. **派生ドキュメントの更新**: CLI 表面仕様の変更に伴い、README、`docs/rules/scope.md`、`docs/specs/jsonl-schema.md` 等の更新が含まれているか。実装差分とドキュメント・テストが矛盾していないか。
21. **plan 内の記述一致**: 設計判断セクションと構造体定義・実装ステップ間で型・値・前提条件が矛盾していないか。段階的に書いた plan は前半の記述が陳腐化しやすい。

上記に該当しないが somniloq 固有の設計判断に関わる問題も自由に指摘してよい。
