---
name: review-plan-all
description: プランレビューの全チェーンを実行する。IMPORTANT: プランモードでプランの記述が完了したら、ExitPlanMode を呼ぶ前に必ずこのスキルを実行すること。ExitPlanMode を直接呼んではならない — 先にこのスキルでレビューを通す。プランを書き終えた、レビューに進む、ExitPlanMode する、といった文脈で自動的にこのスキルを起動する。個別のレビュースキル（/review-plan, /review-plan-codex 等）を直接呼ばず、このスキルを使う。
argument-hint: [plan-file-path]
---

# Plan Review — Full Chain

プランレビューの全ステップを順次実行し、指摘の反映ループを回す。
**各ステップは前のステップの完了を待ってから実行すること。同時実行は禁止。**
**明示的に「ユーザーに確認」と記載されたステップ以外は、ユーザー確認なしで次のステップへ自動的に進む。**

ユーザーが codex スキップを指示している場合、手順 6-7 をスキップする。

## 手順

### 0. `/review-plan-split` を Skill tool で実行する

引数（`$ARGUMENTS`）があればそのまま渡す。プランの粒度を判定する。

戻り値テキストから `^RESULT_FILE: ` / `^SUMMARY: ` 行を抽出する。

- **`SUMMARY: ... needs_action=NO ...`（✅ 分割不要）**: 結果ファイルは Read せず、次の手順 1 に進む
- **`SUMMARY: ... needs_action=YES ...`（⛔ 分割推奨）**: `RESULT_FILE` のパスを Read で読み込み（`/tmp/claude/claude-review-results/` 配下であることを確認）、検出シグナルをユーザーに提示し、`AskUserQuestion` で以下を尋ねる:
  - 選択肢 1: 「backlog に分割して Plan モードを抜ける」
  - 選択肢 2: 「このまま 1 プランで進める（分割しない理由をプランに明記する）」

  選択肢 1 の場合、以降のレビュー（手順 1 以降）は**全てスキップ**し、呼び出し元に「分割のため Plan モードを抜けてください」と伝えて終了する。選択肢 2 の場合は次の手順 1 に進む（プラン本文に「分割しないと判断した理由」を追記してから）。

`RESULT_FILE:` の値が `ERROR` で始まる場合、本文がそのまま戻り値内に含まれているのでフォールバックとして扱う。

### 1. `/review-plan` を Skill tool で実行する

引数（`$ARGUMENTS`）があればそのまま渡す。

### 2. `/review-plan-go` を Skill tool で実行する

引数があればそのまま渡す。

### 3. `/review-plan-somniloq` を Skill tool で実行する

引数があればそのまま渡す。

### 4. 新規の 🔴 MUST / 🟡 SHOULD 指摘をプランに反映する

| スキル | 出力形式 |
| --- | --- |
| `/review-plan` | **結果ファイル化** |
| `/review-plan-go` | **結果ファイル化** |
| `/review-plan-somniloq` | **結果ファイル化** |

各スキルの戻り値テキストから `^RESULT_FILE: ` 行と `^SUMMARY: ` 行を抽出する。
- `RESULT_FILE:` の値が `ERROR` で始まる場合、本文がそのまま戻り値内に含まれているのでフォールバックとして扱う（戻り値本文を読む）

**指摘反映の進め方**:
1. 全スキル実行が完了するまで結果ファイルは Read しない（戻り値の `RESULT_FILE` / `SUMMARY` 行のみ受け取る）
2. 全スキル完了後、`SUMMARY: ... needs_action=YES ...` のものについて、`RESULT_FILE` のパスを Read で読み込む
   - パスが `/tmp/claude/claude-review-results/` 配下であることを確認してから Read する（パス検証）
   - `needs_action=NO` のスキルの結果ファイルは Read しない
3. 全指摘を一覧し、🔴 MUST / 🟡 SHOULD 指摘の対応方針を決定する。対応方針の判断は呼び出し元 workflow（`rules/workflow/1c-plan.md`）の「レビュー指摘への対応」「レビューの収束条件」に従う
4. 対応方針が決まったら Edit に入る。隣接セクションへの修正は 1 つの Edit にまとめる。離れたセクションへの修正は別 Edit のまま（diff レビュー粒度を保つため）
5. 反映完了後、結果ファイルは再 Read しない（古い Read 結果は履歴から自然に流れる）

判断が必要な指摘は AskUserQuestion でユーザーに確認する。

### 5. 新規指摘があった場合 → 手順 1 に戻る

新規 MUST/SHOULD がゼロになるまで codex 以外のレビュー群（手順 1〜3）でループする（手順 0 には戻らない。粒度判定は初回のみ）。

### 6. `/review-plan-codex` を Skill tool で実行する

**引数は自然言語で渡すこと**（harness 制約により `--resume` や `--flag` 形式のフラグは `$ARGUMENTS` 全体を空にしてしまう）:

- 初回: `args: "プランファイル <PLAN_PATH> をレビューしてください"`
- 2 回目以降: `args: "プランファイル <PLAN_PATH> を前回の続きで再レビューしてください"`

`<PLAN_PATH>` は手順 1 で受け取ったプランファイルの絶対パスに置換する。

戻り値テキストの 1 行目は `plan mode: ...` のカナリア。続く行から `^RESULT_FILE: ` / `^SUMMARY: ` を抽出する:

- **`RESULT_FILE:` 行が存在し `SUMMARY: ... needs_action=YES ...` の場合**: `RESULT_FILE` のパスを Read で読み込み（`/tmp/claude/claude-review-results/` 配下であることを確認）、指摘を確認した上で `/codex-findings-append` を実行する。引数: `plan somniloq "<変更概要>"`
- **`RESULT_FILE:` 行が存在し `SUMMARY: ... needs_action=NO ...` の場合**: LGTM。結果ファイルは Read せず、`/codex-findings-append` も呼ばない
- **`RESULT_FILE:` 行が存在しない場合**: 失敗系（PLAN_PATH 抽出失敗・タイムアウト・再試行失敗）。戻り値本文をそのまま読み、ユーザーに状況を報告する
- **`RESULT_FILE:` の値が `ERROR` で始まる場合**: 書き出し失敗のフォールバック。戻り値本文を直接読む

### 7. 新規指摘があった場合 → 手順 1 に戻る

Codex の指摘を反映した場合、整合性を取るためにもう一度 codex 以外のレビュー群（手順 1〜3）を全部走らせる。

Codex レビューが 2 回目の場合、自動で反映せず指摘内容をユーザーに提示する。ユーザーがさらにループするか終了するか判断する。

### 8. 新規指摘なし → 完了

「プランレビュー完了。ExitPlanMode で承認を求めてください。」と報告する。
