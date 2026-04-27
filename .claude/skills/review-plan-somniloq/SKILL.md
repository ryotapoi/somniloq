---
name: review-plan-somniloq
description: somniloq 固有の設計制約に基づくプランレビュー。通常はチェーンスキルから呼ばれる。
argument-hint: <plan-file-path>
allowed-tools: Read, Glob, Grep, Bash(mkdir:*), Bash(printf:*), Bash(date:*), Write, Task
context: fork
model: opus
effort: xhigh
---

# Self Plan Review — somniloq Project

グローバルの `/review-plan` の後に追加実行する somniloq プロジェクト固有のプランレビュー。
1つの Plan サブエージェントで実行する。

**重要な制約:**
- レビューは Task ツール（subagent_type: Plan）で実行する。自分で直接レビューしない

## 手順

### 1. プランファイルのパスを決定する

- `$ARGUMENTS` が空でなければ、その値をプランファイルパス `PLAN_PATH` とする
- `$ARGUMENTS` が空なら「プランファイルのパスを引数で指定してください（例: `/review-plan-somniloq path/to/plan.md`）」と返して終了する

### 2. プランファイルを読む

- Read で `PLAN_PATH` を読み込む
- プラン内で参照されているファイル（仕様書・対象コード）のパスを抽出する

### 3. Plan サブエージェントを起動する

Task ツールで `subagent_type: Plan, model: "claude-sonnet-4-6"` を使う。

エージェントには以下を渡す:
- プランの全文
- 参照ファイルのパス一覧
- 「コードや仕様書は自分で Read/Grep/Glob して確認すること」という指示

#### Agent 1: somniloq 固有の設計制約チェック

プロンプト:

```
あなたはコードレビュアーです。以下の実装計画を「somniloq プロジェクト固有の設計制約」と照合し、違反がないか検証してください。

## 実装計画
{PLAN_CONTENT}

## 検証手順
1. プラン内で参照されている対象コードを Read で読む
2. 以下の設計制約リストとプランを照合する
3. 違反があれば指摘する

## somniloq 設計制約

以下はこのプロジェクトで守るべき設計上の制約です。プランがこれらに抵触していないか検証してください。

### モジュール配置・構造
1. **モジュール配置は依存方向 `cmd/somniloq → internal/core` に従う**: 新しいコードの配置先が rules/architecture.md の責務定義と合っているか。定義された方向に違反する依存がないか
2. **共通化は依存方向に沿って配置する**: `cmd/somniloq` と `internal/core` 間で共有するコードは `internal/core` に置く。片方だけ変更したくなったとき分離できるか検討されているか
3. **リファクタリングと機能実装を同一ステップに混ぜない**: 既存コードの構造改善が必要なら、機能実装の前ステップとして分離されているか

### DB・SQL の安全性
4. **SQL プレースホルダの使用**: JSONL 由来のデータは必ず `?` プレースホルダ経由で SQL に渡す。文字列結合で SQL を組み立てない
5. **modernc.org/sqlite の LastInsertId の罠**: `ON CONFLICT DO NOTHING` 時、`LastInsertId()` は前回挿入の rowid を返す。`RowsAffected()` を先にチェックすること

### プラン内部の整合性
6. **既存データへの影響考慮**: フィルタ・スキップ・バリデーションの削除や変更時、既存 DB に保存済みのデータへの影響が検討されているか。「新規データだけ正しくなる」で済まない場合がある
7. **プラン内の記述一致**: 設計判断セクションと構造体定義・実装ステップ間で型・値・前提条件が矛盾していないか。特にプランを段階的に書いた場合、前半の記述が後半の変更で陳腐化しやすい
8. **既存テストとの対称性**: 既存の類似機能にあるテスト観点（例: ミリ秒境界テスト）が、新機能のテスト計画にも含まれているか。片方だけテストがあると退行を検出できない

### ドキュメント・仕様の同期
9. **scope.md / architecture.md との整合**: 新機能や外部依存の追加時、仕様ドキュメントの更新がプランの変更ファイル一覧に含まれているか
10. **Usage / ヘルプ文字列の更新**: フラグ追加・モード追加・排他的引数パターンの変更時、usage 定数やヘルプ文字列の更新がプランに含まれているか
11. **派生ドキュメントの更新**: CLI 表面仕様の変更に伴い、README.md, README.ja.md, examples/ 配下の SKILL.md 等の派生ドキュメントの更新がプランに含まれているか

### 設計判断の副作用
12. **互換性への影響の明示**: 出力フォーマットの変更が既存の利用パターン（スクリプト連携、TSV パース等）に影響する場合、破壊的変更として明示されているか
13. **集計キーと表示キーの整合**: GROUP BY キーと表示の短縮・変換で情報が縮退し、同名行が出現しないか。集計キーも合わせて寄せる必要がないか検討されているか

上記に該当しないが somniloq 固有の設計判断に関わる問題も自由に指摘してよい。

## 出力形式
- 日本語で出力
- 指摘事項は箇条書きで、該当するコード・計画の箇所を引用する
- 指摘ごとに重要度を付ける: 🔴 MUST / 🟡 SHOULD / 🔵 NIT
- 問題がなければ「somniloq 固有の指摘なし」と記載する
```

### 4. 結果ファイルを書き出して返り値を返す

1. 結果保存先ディレクトリを作成: Bash で `mkdir -p /tmp/claude/claude-review-results`
2. 結果ファイル名を組み立てる: Bash で

   ```bash
   printf '%s/review-plan-somniloq-%s-%04x.md' \
     /tmp/claude/claude-review-results \
     "$(date +%Y%m%d-%H%M%S)" \
     "$RANDOM"
   ```

   出力されたパスを `RESULT_PATH` とする
3. Write ツールで `RESULT_PATH` に検証結果本文を書き出す（フォーマットは下の「結果ファイルの中身」参照）
4. 集計: 🔴 MUST / 🟡 SHOULD / 🔵 NIT の件数を数え、`must`, `should`, `nit` の値を決める。`needs_action` は `must + should > 0` なら `YES`、それ以外は `NO`
5. ユーザーに返す text は以下の 2 行のみ:

   ```
   RESULT_FILE: <RESULT_PATH>
   SUMMARY: needs_action=<YES|NO> must=<N> should=<N> nit=<N> — <1行サマリ>
   ```

   `<1行サマリ>` は LGTM 時は「somniloq 固有の指摘なし」、指摘ありの場合は最重要指摘の要旨を 1 行で。検証本文は text に貼り付けない。

#### フォールバック

mkdir / Write のいずれかが失敗した場合、以下の形式で text を返す:

```
RESULT_FILE: ERROR — <失敗理由を1行で>

<従来形式の検証本文（下の「結果ファイルの中身」と同じ構造）>
```

main 側は `RESULT_FILE: ERROR` を検出したら本文を直接読む。

### 5. 結果ファイルの中身

`RESULT_PATH` に書き出す検証本文は以下の形式:

```
## 自己レビュー結果（somniloq 固有）

### somniloq 設計制約チェック
{Agent 1 の結果}
```

