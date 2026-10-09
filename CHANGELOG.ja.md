# 変更履歴

[English](CHANGELOG.md) · 日本語

## v0.14.1 — 2026-10-09

### 変更

- Codex の import と移行で、同じ cwd の repository 判定を処理 pass 内で再利用し、Git の反復呼び出しを削減した。後続の実行では判定をやり直す。
- 移行時は所属証拠を先に保持し、本文を会話単位で読み込むことでメモリ使用量を削減した。strict 検証と保存処理で JSONL 解析結果を共有し、snapshot 検証は全文を再確保せず再利用 buffer で hash を計算する。ピークメモリは引き続き最大会話の大きさに依存する。
- Codex の import と移行で、重複 payload ID を保持済みメッセージと直接比較し、全文 JSON 署名の複製をなくした。完全一致による重複排除、衝突拒否、発言番号は維持する。
- 合成入力による移行の性能計測と改善候補の評価を記録した。所属検証、snapshot 変更の検出、group 失敗時の既存データ保持、再実行の挙動を維持し、CLI・設定・DB 形式は変更しない。

## v0.14.0 — 2026-10-08

### 追加

- source ごとの複数ログ root を指定する TOML 設定と `config init` を追加。DB コマンドは `--config` 省略時に名前付きの `default` 設定を使い、設定生成時は設定ファイルまたは参照 DB が既存なら拒否する。
- 入力ごとに分離した会話 identity と完全な `slq1:...` REF を追加。別入力の同名 session ID を分け、Claude Code の子・孫も root と session ID を共有していても独立した原文を保持する。
- 対応する旧 DB の固定 standalone snapshot を別の移行先へコピーする `migrate` を追加。残存ログで本人への帰属を証明できた Codex 履歴だけを置換し、他の履歴は legacy REF で参照できる状態で保持する。通常コマンドは旧形式・非対応 schema を変更せず拒否する。[移行手順](docs/migration.md)を参照。
- 本人と確定子孫内の正規表現全一致検索を追加。複数 `-e`、`-F` の固定文字列、`--all` の AND 照合に対応し、原文の UTF-8 byte 位置と `show` に共通する発言番号を返す。
- 外部ソフトウェアと Go のライセンス通知、生成スクリプト、CI の整合性確認を追加。README 両言語に CI バッジと言語切り替えを整備した。

### 変更

- `sessions` をまとまり単位の `search` 一覧へ統合。既定で全件を返し、任意のページングと入力・source・project・活動日・取り込み日時の条件に対応する。一覧は本文抜粋を含まず完全 REF と metadata を返し、活動日の mode は `active`・`started`・`last`・`overlap` とする。
- `show` を会話の原文取得へ再設計。複数の完全 REF、任意の確定子孫展開、role・発言番号・日時の条件、ページング、末尾選択、一行表示に対応し、原文 blocks と番号を保持する。
- `search` と `show` の JSON を `{items,total,count,limit,offset,hasMore,nextOffset}` envelope に、TSV をページ情報と可逆な field escape を含む形式に変更。`projects` の JSON は配列を維持する。
- 旧 JSON 設定を TOML に置き換え、既存 JSON の探索・自動変換は行わない。裸の session ID・短縮 REF を拒否し、`outline`、`show --summary`、turn 選択、表示除外フラグ・設定、Markdown 出力、REF なしの期間取得を廃止した。更新前にスクリプトと設定を変更する必要がある。
- 同じ cwd の Claude repository 解決を再利用し、移行時の旧履歴読取を関係する UUID に限定し、複数 REF の `show` で関係解決を共有して反復処理を減らした。

### 修正

- Codex の本人原文を継承文脈と分けて保持し、複数 rollout の発言順と親 metadata の後着に対応した。
- 原文が不変な Claude 会話の取り込み日時を保持し、Codex rollout の metadata-only 編集後も本文を再構築して通常・全件取り込みの結果を揃えた。
- Codex の親 metadata が循環しても正常本文を保持し、循環する関係だけを未確定として扱うようにした。
- 同時に実行した通常取り込みの古い snapshot が、新しい本文・cursor・取り込み日時を上書きする問題を防いだ。
- 移行再実行時に以前取り込んだ rollout が欠けていても、移行済み会話の本文と cursor を保持するようにした。

## v0.13.0 — 2026-10-04

### 変更

- `outline` と `show --summary` の user message 表示除外を、trim した本文全文に適用する任意の `excludeUserMessagePatterns` 設定へ統一した。繰り返し指定する `--exclude-user-message-pattern` はその呼び出しで設定値を置き換え、`--no-exclude-user-messages` は除外を一時的に無効化する。`outline` は除外後も元の turn 番号を保ち、summary は除外後の先頭 N 件を選ぶ。
- `show --summary` は `/clear` と command caveat を既定では除外しなくなり、`--include-clear` を廃止した。旧 `commandPatterns` 設定は自動移行せず、`sessions` の非コマンド user turn 件数と先頭行のヒントも廃止した。TSV は `source` を最後に置く 8 列となり、JSON から `nonCommandUserTurnCount` と `firstNonCommandUserLine` を削除した。
- `backfill` コマンドと旧 DB 専用の移行・補正コードを廃止した。一般的な schema 管理は続けるが、v0.3 形式の DB から現在の source 付き schema への移行成功は保証しない。
- source と session ID によるメッセージ索引を追加し、複数セッションの本文取得で全メッセージを繰り返し走査する負荷を減らした。既存 DB を開く際にも索引を作る。

### 修正

- 差分 import と全件 import が並行したとき、古い取り込み位置によって保存済み本文が欠落したまま成功扱いになる問題を防いだ。
- 新規 DB ファイルと新規作成する親ディレクトリを所有者限定の権限にした。既存のファイル・ディレクトリの権限は変更しない。
- Claude Code・Codex の repository 判定で親プロセスの `GIT_*` 環境変数を無視し、別 repository の Git 設定による project の誤認を防いだ。保存済み path は自動更新しない。
- `sessions` TSV の保存済み不正 timestamp にタブ・改行があっても、1 セッション 1 行・8 列を保つようにした。JSON は元の文字列を維持する。
- 時刻 filter が RFC3339 の数値 offset で 24 時以上、または分が 60 以上の値を拒否するようにした。
- Markdown の link 診断が basename の競合・存在しない参照先・壊れた anchor を報告したとき、診断コマンド自体が成功しても CI を失敗させるようにした。
- search の文書で flag を検索語より前に置き、CLI が受理する構文に合わせた。

## v0.12.5 — 2026-10-02

### 変更

- `search` の turn 番号を付ける際、対象 session の全メッセージ本文を追加取得せず、メッセージ ID と role だけを取得するようにした。検索結果、snippet、turn 番号は従来どおり。

### 修正

- Claude Code の user / assistant レコードで `sessionId` または `uuid` が欠落・空の場合、空 ID で保存せず unparsed として診断するようにした。前後の正常なレコードは引き続き取り込まれ、保存済みデータは自動修復しない。

## v0.12.4 — 2026-10-01

### 変更

- CLI、DB クエリ、取り込み処理の内部構造を、既存の挙動を保ちながら簡素化した。重複するテストも整理し、公開コマンドと取り込みの契約に対する検証は維持した。

## v0.12.3 — 2026-10-01

### 修正

- 差分 import が、追記中の JSONL の未完成な最終行をファイルの伸長後に再試行するようにした。改行のない完成済み最終行は従来どおり取り込む。
- Codex の `session_meta` で ID が欠落・空の場合、後続メッセージを空 ID のセッションに混ぜず、unparsed として診断するようにした。
- 保存済み timestamp が不正な場合は時刻比較で未知として扱い、1 件の不正値が `sessions`、`projects`、`show`、`search` で正常なデータを読む妨げにならないようにした。保存値と JSON 出力には元の値を残す。
- 実在する Git linked worktree の Claude Code・Codex セッションを、project 一覧と filter で本体 repository に集約するようにした。保存済みの非 NULL path を更新するには全件再取り込みが必要。
- `sessions` と `import` が予期しない位置引数をエラーにするようにした。後続の flag が黙って無視される場合も防ぐ。
- date-only の時刻 filter と `logical_day` が、夏時間の切り替えを跨いでも設定したローカル時計の境界を使うようにした。
- `sessions` と `projects` の TSV project 名に含まれるタブ・改行を空白に置換し、列と行の境界を保つようにした。JSON の値は変更しない。

## v0.12.2 — 2026-10-01

### 変更

- 重複するテストを統合し、コマンド出力と取り込みの回帰検証を維持した。CLI と保存データの挙動は変わらない。

## v0.12.1 — 2026-09-24

### 変更

- `imported_at` は、その session を最後に保存または更新した JSONL ファイル処理の開始時刻であり、変更なしとして処理したファイルでは更新されないことを文書で明確にした。
- ソースからのビルドに Go 1.27.1 以降が必要になり、CI も Go 1.27.1 を使用する。

### 修正

- `import --full` で `messages`、`sessions`、`import_state` の全行を1つの transaction で削除するようにした。削除に失敗した場合も3表の既存データを保ち、一部だけ削除された状態にならない。
- 保存された RFC3339 timestamp を文字列ではなく時点として比較し、メッセージ・セッション・プロジェクト・検索結果の順序と、session の開始・終了時刻の選択を正しくした。UTC offset や小数秒の精度の違いで時系列が誤らず、メッセージの同時刻順は従来どおり `rowid` で決まり、空の開始時刻は有効な時刻で補われる。
- `import` が stderr への診断出力に失敗した場合、成功終了せずエラーを返すようにした。

## v0.12.0 — 2026-09-23

### 追加

- `sessions`、`projects`、`show`、`search` の `--since` と `--until` が、秒・小数秒・`Z` または数値の UTC offset を含む RFC3339 instant を受け付けるようになった。`sessions --imported-since` も同じ形式を受け付ける。明示した offset により、実行時のタイムゾーンにかかわらず同じ時点を指定できる。

### 変更

- 検索クエリと、`sessions`、`show`、`search` の `--project` で、`%`、`_`、`\` をワイルドカードではなくリテラルな文字列として扱うようにした。既存の部分一致、ASCII の大文字小文字を無視する挙動、project alias の展開は維持し、DB migration、backfill、再取り込みは不要。

### 修正

- RFC3339 の時刻境界が小数秒を保持して正確に比較され、等価な UTC offset を同じ時点として扱うようにした。`--since` の境界は含み、`--until` の境界は含まない。

## v0.11.0 — 2026-09-21

### 追加

- `show` と `outline` が `--source` を受け付けるようになった。同じ session ID が複数の source に存在する場合、特定の import source からセッションを開ける。未指定時は従来どおり曖昧性エラーとなる。
- `search --format json` が、source、session ID、turn、timestamp、project、検索に一致したスニペットを含む機械可読な結果を出力するようになった。
- `search --limit` と `--offset` が、既存の検索順を維持したまま結果をページ分割するようになった。ページネーション指定がない場合は、従来どおり全件を返す。
- `sessions --imported-since` が、指定時刻以降に取り込まれたセッションを一覧できるようになった。会話の timestamp が不明なセッションも含まれる。

## v0.10.1 — 2026-09-20

### 修正

- `import --full` と `backfill` で、確認プロンプトの読み書きに失敗した場合は確認拒否として扱わず、破壊的処理の前にコマンドエラーとして停止するようにした。

## v0.10.0 — 2026-09-16

### 追加

- `~/.cursor/projects/` 配下の Cursor Agent transcript を、デフォルトまたは `--source cursor-agent` で取り込めるようにした。

### 変更

- `sessions` / `search` の TSV 出力と `show` の Markdown 出力で、各セッションの source を識別できるようにした。`show` / `outline` は、同じ session ID が複数 source に存在する場合に候補 source も表示する。
- timestamp または repository metadata がないセッションも、フィルタなしでは表示するようにした。time / project filter には一致せず、両方の timestamp が不明な場合は time range を空で表示する。

## v0.9.1 — 2026-09-06

### 修正

- CLI コマンドで標準出力への書き込み失敗を検出し、成功扱いせずエラー終了するようにした。対象は `import`、`backfill`、`outline`、`projects`、`search`、`sessions`、`show`。

## v0.9.0 — 2026-07-24

### 修正

- DB query のエラーに、操作名と段階（query / scan / iteration）を付けるようにした。原因エラーのラップと既存の not-found 挙動は維持する。

## v0.8.1 — 2026-07-23

### 変更

- config を読むサブコマンドの flag 宣言を統合し、help の事前判定で使う flag の認識と値消費判定を、実際の `FlagSet` 定義から導くようにした。
- 未使用の `ingest.Adapter.Source` メソッドを削除した。import source identity は引き続き import source の登録と JSONL 処理で決まる。

## v0.8.0 — 2026-07-12

### 追加

- `import` が parse / normalize に失敗した先頭5件までを、`file:line: error` 形式で stderr に表示するようになり、スキーマ変更の原因を追いやすくなった。既存のサマリ件数、差分取り込みの offset、終了コードの意味は変わらない。

### 変更

- import の transaction 生成、source 別 normalize、query filter、migration / backfill、CLI 整形を共通化し、source 横断の解決、時刻境界、出力スキーマ、import / migration 境界の回帰テストを拡充した。

### 修正

- 永続化に失敗した行を、本文を書き込めた行として数えないようにした。

## v0.7.2 — 2026-07-04

### 変更

- source 共通のメッセージ永続化と CLI / import の補助処理を共通化し、両方の import adapter の保存動作を揃えた。

## v0.7.1 — 2026-07-04

### 変更

- core の DB 責務を schema、write、query、connection の層に分割し、backfill の DB アクセスを DB execer 経由に統一した。
- schema の同等性、migration 競合の再確認、backfill で解決できないパス、PRAGMA 復元失敗、縮小されたファイルの import に対する回帰テストを拡充した。
- ADR 0008 にある `core` → `claudecode` の限定的な依存例外を文書化した。

## v0.7.0 — 2026-07-04

### 追加

- `sessions` が、コマンドでない user turn の件数と先頭行をスキップ判断用ヒントとして出すようになった。`commandPatterns` でコマンド扱いする turn を設定できる。
- 論理日を追加した。`dayBoundary` の設定または `--day-boundary` で `sessions` / `search` の date-only フィルタの基準を指定でき、`sessions` に `logical_day` 列が加わった。
- `outline` が各 turn の本文合計サイズ（応答を含む）を出すようになった。
- 検索結果に、`outline` / `show --turn` と共通の turn 番号を追加した。
- project alias に一致する表示を canonical 名に統一し、`projects` では alias 同士の行をまとめるようにした。`--short` は alias 非一致の project だけを短縮する。
- サブコマンドの help に、出力スキーマ、挙動の注意点、例を追加した。

### 修正

- Codex のルート走査失敗をエラーとして報告するようにした。一方、存在しないルートは未使用 source のまま扱い、子孫の走査失敗は非致命のままとした。
- 壊れた config があっても、`--help` と config 不要のコマンドが失敗しないようにした。サブコマンド間の usage error 整形も統一した。

## v0.6.0 — 2026-06-11

### 追加

- 長いセッションを user-message turn 単位で俯瞰する `outline` を追加した。
- セッションの一部だけを読む `show --turn` と `--tail` を追加した。
- `sessions` 出力に `body_size` を追加した。
- `sessions`、`show`、`projects`、`outline` の JSON 出力を追加した。
- セッション横断検索の `search` を追加した。
- リポジトリ名の変更をまたいで `--project` を展開する `projectAliases` 設定を追加した。

## v0.5.0 — 2026-06-11

### 変更

- `import` のサマリに `unparsed lines` 件数を追加した。サマリを解析するスクリプトは新しい項目に対応する必要がある。
- ディレクトリ走査エラーを非致命にした。読めないディレクトリをスキップして他のファイルを取り込み、エラーは stderr に表示し、エラーが1件でもあれば終了コードは1になる。存在しない source ディレクトリは未使用 source として扱う。
- JSONL 取り込み骨格、サブコマンドの終了コード処理、v0.3 → v0.4 migration の事前条件、SQL 層の sidechain フィルタを共通化し、DB テストを関心ごとに整理した。

## v0.4.0 — 2026-05-05

### 追加

- Claude Code ログに加えて Codex rollout JSONL を正式な取り込み source として追加した。
- `somniloq import --source all|claude-code|codex` と、両 source を横断する `sessions` / `projects` 表示を追加した。
- `backfill` が v0.3 データベースの v0.4 スキーマ移行も行うようになった。

### 変更

- セッションキーを `session_id` だけでなく `(source, session_id)` にした。v0.3 のデータベースは v0.4 で import する前に `somniloq backfill` を1回実行する必要がある。
- `import` は Claude Code と Codex の両方をデフォルトで取り込むようになった。`--source` で adapter を絞り、`--full` は source 指定時でもデータベース全体を削除する。

## v0.3.0 — 2026-05-02

### 追加

- `somniloq backfill` を修復処理の入口として追加した。欠けた `repo_path` を解決し、v0.2.x 由来の孤立セッションを削除する。削除前に確認し、`--yes` で省略できる（破壊的な非対話実行では必須）。

### 変更

- `projects` の集約と `--project` フィルタを `repo_path` のみにした。`project_dir` 列を削除し、古い行は `--project` に一致させる前に `somniloq backfill` が必要になった。
- Git 解決に失敗した場合の `ResolveRepoPath` は `cwd` にフォールバックし、Git 管理外で開始したセッションにも安定したキーを与えるようになった。
- `import` はメタデータだけのレコードからセッションを作らず、会話レコード（`user` / `assistant`）がある場合だけ作るようになった。

## v0.2.1 — 2026-04-22

### 変更

- `--summary N` は件数を受け取り、各セッションの先頭 N 件の user message を表示するようになった（未指定または 0 で無効）。v0.1.x の boolean フラグからの破壊的変更。
- `--summary` はデフォルトで `/clear` の echo と `<local-command-caveat>` ブロックをスキップするようになった。残す場合は `--include-clear` を使う。
- `--summary` が session ID 指定と時間範囲指定の両方で動作するようになった。

### 修正

- `show` の usage 表示で、Go の flag 解析順に合わせて `<session-id>` より前にフラグを置くよう修正した。

## v0.1.1 — 2026-04-02

### 追加

- build 情報に基づく `--version` フラグを追加した。

## v0.1.0 — 2026-04-01

### 追加

- Claude Code のセッションログを import / search する somniloq CLI の初回リリース。
- `~/.claude/projects/` の JSONL ファイルを差分取り込みして SQLite に保存する `import`。
- 時刻・プロジェクトで絞り込み、TSV で出力するセッション一覧。
- セッション数を表示するプロジェクト一覧。
- `show` によるセッション内容の Markdown 表示。
- すべての入力・出力でローカルタイムゾーンをサポート。
- Git worktree のセッションを正規化。
- プロジェクト名を短縮する `--short`。
- セッション概要を素早く確認する `--summary`。
