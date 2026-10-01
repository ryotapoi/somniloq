# Backlog

## 運用ルール

- Backlog には、今後着手でき、完了を判定できる作業を `- [ ]` 形式のタスクとして置く。
- 対応要否を決める調査もタスクにできる。観察事実だけを残さず、判断したい問いと、判断をもって完了とすることを明示する。
- 粗いメモで起票してよいが、着手前に目的と受け入れ条件を読める粒度へ詰める。
- バージョン番号を付ける場合は SemVer に従う。
- タスクは `###` 見出しでグループに分ける。バージョンが確定したグループだけ、`### v0.11.0 名前` のように見出しへ番号を付ける。
- グループが大きい場合は、`####` 見出しでさらに分割してよい。

## タスク

### v0.12.2 テストの簡略化

- [x] `principle-test-verification-policy` に沿って既存テストの保持価値を見直し、重複・実装詳細への依存・維持コストが便益を上回るテストを削除または統合する。必要な振る舞いと既報不具合の回帰検出は、最も低く安定した検証層で維持し、整理後の検証結果と残る不確実性を確認する。

### v0.12.3 不具合修正

- [x] 追記中の JSONL の未完成な最終行を import した後、同じ行が完成しても差分取り込みで失われないようにする。Claude Code・Codex・Cursor Agent の差分再開位置を確認し、完成済みで改行のない最終行を取り込める既存の挙動も維持する。
- [x] Codex の `session_meta` で `id` が欠落・空の場合、後続メッセージを空の session ID に保存しない。複数 rollout の本文が一つの session に混ざらず、不正な metadata が診断されることを確認する。
- [x] import が保存できる不正な timestamp 1件で `sessions`・`projects`・`show`・`search` が失敗しないようにする。新規取り込みと既存 DB の不正値が、それぞれ他の正常データの参照を妨げないことを確認する。
- [x] `/.claude/worktrees/` 以外の通常の Git linked worktree から取り込んだ会話も、本体 repository の project に集約する。本体と worktree を跨ぐ一覧・project filter を確認する。
- [x] `sessions` と `import` で予期しない位置引数をエラーにする。位置引数の後にあるフラグが黙って無視されず、特に `import --full --yes` の source 制限を誤って外さないことを確認する。
- [x] DST 切り替え日にも `dayBoundary` を指定したローカル時刻として扱う。`--since` / `--until` の日付条件と `logicalDay` が、春・秋の時刻変更を跨いでも同じ境界で一致することを確認する。
- [x] `sessions` と `projects` の TSV 出力で project 名のタブ・改行・CR を処理し、列数と行数を保つ。project alias と repository path 由来の値を確認し、JSON では元の文字列を保持する。
- [x] `simplify-tests` skill を実施する。

### v1.0.0 公開機能の整理

- [ ] `outline` と `show --summary` に、trim 済みの user message 全文へ同じ Go 正規表現を適用する除外を導入する。
  - config の `excludeUserMessagePatterns` を未設定なら除外なしとし、繰り返し指定できる CLI pattern は OR で評価して指定時に config のパターン一覧全体を置き換える。全除外をその呼び出しだけ無効化する指定を設け、override と無効化の併用はエラーにする（空正規表現を無効化の意味にしない）。不正な regex もエラーにする。
  - 除外は first line・件数制限より先に適用し、`outline` は元の turn 番号を維持、summary は除外後の先頭 N 件を表示する。保存 DB、全文 `show` / `--turn` / `--tail`、`search`、`sessions` の一覧には適用しない。
  - summary 固定の `/clear`・caveat 除外と `--include-clear` は新しい除外指定へ移し、移行例を示す。`sessions` 用の `commandPatterns`、slash-prefix 判定、`nonCommandUserTurnCount`、`firstNonCommandUserLine` は維持する。dayBoundary・logicalDay と summary 自体の廃止は含めない。関連する docs・help・README・config/output の移行案内を更新し、設定と CLI の優先順位、共有 matcher と順序、元の turn ID、不正 regex を自動検証する。

### 優先度低（対応要否未確定）

- [ ] 公開の `backfill` コマンドと専用の旧データ修正・移行コードを維持する価値と、削除時の旧 DB 利用者への影響を確認し、削除するか判断する。削除する場合は、元ログを失った旧 DB も含めたアップグレード手順を先に確定し、関連する help・仕様・README・テストを更新する。継続する場合は、並行 import による `repo_path` 更新との競合への対応要否を判断する。新たな自動移行や一般的な schema 管理の削除は前提にしない。
- [ ] `sessions` の `nonCommandUserTurnCount` と `firstNonCommandUserLine` の実利用と維持コストを確認し、削除するか判断する。削除する場合のみ、`commandPatterns` と slash-prefix 判定、関連する出力契約・設定・ドキュメント・テストを整理し、公開出力の変更に適切なリリースを決める。
