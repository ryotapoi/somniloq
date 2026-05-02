# Backlog

## v0.3 — repo_path 設計のやり直し

v0.3 は未リリース。実装途中の commit はそのまま使い、ここから追加コミットで新方針に揃える。

`rules/scope.md` は v0.3 完成時点の SSoT を先取りで記述している（「実装より先に SSoT を更新」のため）。リリース完了までは現状コードとの乖離があり、本セクションの未着手チェックボックス（`[ ]`）で進捗を追う。

### 背景

`sessions` テーブルが「`project_dir` で集計」と「`repo_path` で集計」を同居させている。worktree 配下で別 repo を触ったセッションが 1 件でも混じると、1 `project_dir` 内に複数 `repo_path` が並び、メタセッション（`custom-title` / `agent-name` 由来で `cwd` を持たない行）の集約先をどれに寄せても誤集約が起きる。

### 新方針

1. **会話のあるセッションだけ DB に保存する**。`custom-title` / `agent-name` 単独のメタセッションは保存しない（`cwd` を持たないので `repo_path` 解決手段がない）
2. **`ResolveRepoPath` の手順 4 を「cwd を返す」に変える**。git 配下外でも cwd 自体を一意キーとして採用
3. **`projects` の集計キーを `repo_path` 一本に絞る**。`project_dir` フォールバックは削除
4. → 同 `project_dir` 内 `repo_path` 持ち / NULL の混在も、メタセッション分裂も発生しなくなる

### 役割分担（migration vs backfill）

- **`OpenDB` 起動時 migration（自動・冪等）**: スキーマ操作のみ（`repo_path` カラム追加は既に存在）。データを破壊しない
- **`somniloq backfill`（ユーザー明示実行）**: 既存 DB の v0.2.x 由来データ歪みを正す処理を全部ここに寄せる。バックアップ案内を README に明記

### タスク（上から順に実装・コミット。まとめて v0.3.0 としてリリース）

- [x] **`rules/scope.md` の更新**（実装より先に SSoT を更新）
  - `projects` 集計キーを「`repo_path` 一本」に
  - 「同一 `project_dir` のセッション群で一部だけ `repo_path` 解決済み・残り NULL」の Known limitation を削除
  - メタセッション（cwd なし）が DB に保存されない仕様を追記
  - `backfill` の役割（v0.2.x からのアップグレード時に 1 回叩く / `messages` を持たない行の DELETE と `repo_path` 補完を兼ねる）を明記

- [x] **`ResolveRepoPath` の手順 4 を「cwd 返却」に変更**
  - `internal/core/repo_path.go`: 手順 4 を「`cwd` をそのまま返す」に変更（現状は空文字）
  - 関数コメントの評価順を更新
  - `internal/core/repo_path_test.go`: 手順 4 のケース（git 配下外の一時ディレクトリ）の期待値を「cwd 自体」に変更

- [x] **import 時のメタ前置 INSERT を削除（会話レコードのみが session 行を作る）**
  - `internal/core/db.go` `updateSessionTitle` / `updateSessionAgentName`（`db.go:161-174` / `db.go:180-193`）の前置 INSERT を削除し、UPDATE のみにする（行が無ければ何もしない）
  - JSONL 内では `custom-title` が `user`/`assistant` より前に出現するケースがあるため、`internal/core/import.go` の `processFile` 内で `custom-title` / `agent-name` をいったんバッファに溜め、ファイル末尾で UPDATE を流す
  - テスト: タイトルだけで発話なしのセッションは sessions 行が作られないこと、会話 + タイトルの順序が逆でも `custom_title` / `agent_name` が正しく入ること
  - 採用方針: `messages` 0 件は backfill で DELETE する（取り込み側に text 非空判定を増やさない）。判断理由は ADR 0003 の更新（下記ドキュメント更新タスク）で記述する

- [x] **`projects` 集約と `--project` フィルタを `repo_path` 一本に絞る**
  - `internal/core/db.go` `ListProjects`: `GROUP BY` を `repo_path` 一本にする。`project_dir` フォールバック（`projectDirNormalized` の SQL 分岐）を削除
  - `internal/core/db.go` `ListSessions` の `--project` フィルタも `repo_path` 単独 substring に切り替える（現行は `repo_path` と `project_dir` の OR マッチ）。`show --project` も同じ `ListSessions` を通るので一括で切り替わる
  - `ProjectRow.ProjectDir` が不要になったら struct から削る
  - `cmd/somniloq` 側の表示ロジックも `repo_path` 前提で簡略化（`resolveDisplayName` のフォールバックは削除可）
  - 既存の `projects_test.go` / `sessions_test.go` / `format_test.go` / `shorten_test.go` を新仕様に合わせて更新

- [x] **`backfill` を「データ補正の単一窓口」に拡張**
  - `BackfillRepoPaths` 内で以下を順に実行する:
    1. `messages` 行が 1 件も無い `sessions` 行を DELETE（v0.2.x 時代に作られた cwd NULL のメタセッション残骸を消す）
    2. `repo_path NULL AND cwd 持ち` の行を `ResolveRepoPath` で埋める（手順 4 が cwd 返却になったので全件解決される）
  - 関数名は `BackfillRepoPaths` のままでもいいし、役割が広がるなら `Backfill` 等にリネーム検討
  - 戻り値に「削除した行数」「解決した行数」を含める（CLI で報告）
  - DELETE は破壊的なので、対象が 1 件以上あるときのみ起動時に件数を表示し確認プロンプトを出す（デフォルト No、`--yes` でスキップ可能、import の `--full` と同じ作法）。0 件なら無確認で進む
  - 非対話環境（パイプ・CI 等）では DELETE 対象 1 件以上のとき `--yes` 必須。CLI 配線テストで非対話判定の挙動を担保する
  - pass-2（同 project_dir からの引き継ぎ）は導入しない
  - テスト: `backfill_test.go` を更新
    - 「git 配下外 cwd は cwd 自体が入る」（手順 4 変更）
    - 「`messages` を持たない sessions 行が DELETE される」
    - 冪等性（2 回叩いても同じ結果）

- [x] **`project_dir` カラムを完全撤去する**
  - クエリ側から外しても書き込みだけ残すのは中途半端なので、書き込み・カラム自体まで一気に消す
  - `internal/core/db.go`: `CREATE TABLE sessions` の `project_dir` カラム削除、`INSERT` から `project_dir` を外す
  - migration: 既存 DB に対して `ALTER TABLE sessions DROP COLUMN project_dir` を `OpenDB` 起動時 migration に追加（SQLite 3.35+ 必須。最低バージョン要件を README に明記）
  - `ProjectRow` / `SessionRow` 等の struct から `ProjectDir` フィールドを削除
  - 関連テスト・テストフィクスチャから `project_dir` を削除
  - `rules/scope.md` の `sessions` テーブル DDL から `project_dir` 行を削除

- [ ] **ドキュメント更新**
  - `README.md` / `README.ja.md`: アップグレード手順を追記
    - バックアップ推奨（backfill が DELETE を含むため）
    - 新バイナリ → `somniloq backfill` で既存 DB を補正
    - 過去 JSONL を残してある場合の差分コピー手順（`cp -rn /path/to/old-projects/. ~/.claude/projects/` で現行に無い分だけ補充 → `somniloq import --full --yes`）
    - `--project` の挙動が v0.3 で変わる（`project_dir` 対象から外れ、`sessions` / `show` の両方が `repo_path` 単独 substring になる）旨と、v0.2.x からのアップグレードでは `somniloq backfill` を先に叩くべき旨を明記
    - 既存記述の書き換え（旧仕様 `project_dir` 経由フォールバックが残っている箇所。`README.md` と `README.ja.md` の双方に適用）:
      - `--project` の説明から `project_dir` への OR マッチ記述を削除
      - `sessions` / `projects` TSV 出力の `repo_path (or project_dir if unresolved)` 表記を `repo_path` に統一
      - `--short` の `fallback to last hyphen element` 記述を削除し `filepath.Base(repo_path)` のみに統一
  - `examples/skills/somniloq/SKILL.md`: 旧仕様記述の書き換え（`--project` の OR マッチ、`ProjectDir` 列の存在前提、`--short` のハイフンフォールバック）と Quick start の更新
  - `references/knowledge.md` に新方針の知見を追記
  - `decisions/0003-backfill-as-separate-subcommand.md` を v0.3 の役割拡張（`messages` を持たない sessions の DELETE と確認プロンプト追加）に合わせて更新する。または supersede 用の新 ADR を立てる

## 関連ファイル

- `decisions/0003-backfill-as-separate-subcommand.md` — backfill サブコマンド化の ADR（v0.3 で役割再定義が入る）
