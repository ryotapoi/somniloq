# ADR 0014: project alias は設定ファイルの完全一致グループで展開する

## Status

Accepted

## Context

リポジトリをリネームすると `repo_path` が変わり、旧名時代のセッションと新名のセッションが `--project` で同時に引けなくなる（v0.6 backlog 項目）。DB の `repo_path` は取り込み元の事実をそのまま保持したいので、書き換えではなく検索時の解決で吸収したい。解決ルールの置き場所と展開のセマンティクスが論点。

## Considered Options

- **置き場所 A: 設定ファイル `~/.somniloq/config.json`**: DB の外に置き、DB は事実のみ保持。`--config` で差し替え可能
- **置き場所 B: DB 内のテーブル + 管理サブコマンド**: CLI で完結するが、alias の編集に専用コマンド群（add/remove/list）が必要になり、v0.6 の規模に対して過大
- **展開 A: 完全一致でグループ展開**: `--project` の値が canonical 名か旧名に完全一致したときだけグループ全名に展開
- **展開 B: substring 連鎖**: `--project` の substring マッチに alias もかける。どの alias に当たったか予測しづらく、意図しない巨大グループ展開が起きうる

## Decision

We will load project aliases from `~/.somniloq/config.json` (overridable with the global `--config` flag), expand a `--project` value only on exact match against a group's canonical or old names, and OR the expanded patterns in SQL. 設定ファイル方式は編集が `$EDITOR` で済み、管理コマンドの追加実装が要らない。完全一致展開は「いつ展開されるか」が自明で、非一致時は従来挙動（値そのものの substring マッチ）と完全互換になる。

- 形式は `"projectAliases": {canonical: [old, ...]}`。展開は双方向（旧名指定でも同じグループに解決）
- 展開結果は `core.SessionFilter.Projects []string` として渡し、SQL 側で `repo_path LIKE` を OR 結合する（`projectsCondition`）
- ファイル無しは空設定として正常動作、JSON 破損はエラー（typo で alias が黙って無効化されるのを防ぐ）

## Consequences

- `SessionFilter.Project string` は `Projects []string` に変わる（core API の破壊的変更だが、利用者は cmd 層のみ）
- alias 編集の UX はテキストエディタ頼み。管理コマンドが欲しくなったら別 ADR で追加を検討する
- `projects` 一覧の集約キーには alias を適用しない。旧名・新名は別行のまま表示される（集約まで吸収すると「DB は事実のみ」の境界が曖昧になるため、必要になった時点で再検討）
- canonical 名が別グループの旧名にも含まれるような循環・重複定義は検証しない（最初に一致したグループで展開される）。実用上 1 リポジトリ 1 グループで足りるため
