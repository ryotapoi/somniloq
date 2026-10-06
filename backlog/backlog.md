# Backlog

## 運用ルール

- Backlog には、今後着手でき、完了を判定できる作業を `- [ ]` 形式のタスクとして置く。
- 対応要否を決める調査もタスクにできる。観察事実だけを残さず、判断したい問いと、判断をもって完了とすることを明示する。
- 粗いメモで起票してよいが、着手前に目的と受け入れ条件を読める粒度へ詰める。
- タスクの行には短い作業名を書き、説明は空行を挟んだ字下げ段落に分ける。
- バージョン番号を付ける場合は SemVer に従う。
- タスクは `###` 見出しでグループに分ける。バージョンが確定したグループだけ、`### v0.11.0 名前` のように見出しへ番号を付ける。
- グループが大きい場合は、`####` 見出しでさらに分割してよい。

## タスク

### v0.14.0 セッションのまとまりを軸にした検索・閲覧の再設計

合意した方向、実装契約に残る点、元ログの調査根拠は [v0.14.0 参考資料](v0.14.0-reference.md) に置く。複数入力と会話 identity は移行・検索の土台なので先に実装する。以下は実施順のタスクであり、確定した詳細な実装条件と fixture は [契約](../docs/specs/v0.14.0-contract.md) と参考資料の正本リンクを参照する。各実装タスクに対応する検証と必要な正本・usage・README 両言語・skill の更新を含め、[必須 gate](../docs/rules/verification.md) を通す。旧 CLI・JSON の互換性対応は前提にしない。

#### 1. 契約と代表例の確定

- [x] 残る元ログを調べ、取り込み・保存・参照の契約と代表 fixture を確定する

  [確定契約](../docs/specs/v0.14.0-contract.md)、[移行契約](../docs/specs/v0.14.0-migration.md)、[追加調査・fixture対応](../docs/specs/v0.14.0-log-evidence.md) を正本として第1段階を完了。元ログの未知情報は補完せず、実装は第2段階以降、実使用とユーザー最終確認は第7段階に残す。

#### 2. 複数入力と親子ログの取り込み

- [x] TOML 設定と複数入力から共通 DB への取り込みを実装する

  TOML 設定で DB と複数の入力ルートを指定し、同じ DB へ取り込む。DB 操作は `--config` 省略時に default を使い、設定生成・名前指定・エラー案内・path 解決は [複数のログ入力](v0.14.0-reference.md#複数のログ入力)に従う。入力を source と実体 root で識別し、会話・発言・差分状態を入力ごとに分ける。標準ルートと追加ルート、同名 ID、symlink・相対 path、設定欠落・上書き拒否を fixture と CLI で確認する。設定から保存・取得まで動く状態を完了とする。

- [x] 共通保存モデルと Codex の親子取り込みを実装する

  Codex adapter、共通正規化型、SQLite 保存を一緒に変更し、本人 identity・直接親・子孫の独立本文・発言順序と未知日時を保持する。[Codex の継承境界](v0.14.0-reference.md#調査で確認した実装上の条件)を `subagent_history_start_ordinal` のあるログだけに適用し、親 metadata で本人 identity を上書きしない。子先行・親後着、差分再開・再処理、別入力の同名 ID を fixture で確認する。他 source の root も新規 DB で保存・取得できる状態を完了とする。

- [x] Claude Code の子・孫ログを独立した会話として取り込む

  subagents を走査し、root と session ID を共有する子も独立した会話として保存する。[調査で確認した](v0.14.0-reference.md#調査で確認した実装上の条件) Agent call・tool result・構造化 `agentId` の厳密な照合で分かる直接親と、path から分かる root 所属を区別する。未確定の直接親・取得できない文脈は補わず、本人の原文を保持する。兄弟・孫、親後着、差分・再処理、別入力の同名 ID を fixture で確認する。

#### 3. 既存履歴の移行

- [x] 専用 `migrate` で既存履歴を移し、残存ログのある Codex セッションだけ置換する

  [既存 SQLite の扱い](v0.14.0-reference.md#既存-sqlite-の扱い)に従い、既知の旧 DB を別の新 DB へ限定変換し、残存ログで入力・本人 ID が確かめられる Codex 会話だけ全文置換する。旧 UUID が示す物理的出自を会話帰属の証明と混同せず、所属不明・ログ欠落・他 source の本文を保持する。初回コピーと移行元識別、セッション単位の原子的置換・再実行、失敗時の旧本文保持を fixture で確認する。非対応 DB の案内と [ADR 0019](../docs/decisions/0019-remove-legacy-backfill.md) の見直しを正本へ反映する。

#### 4. 会話の参照と原文取得

- [x] REF とセッションのまとまりを共通に解決する

  入力・source・子を一意に指すコピー可能な REF と、保存した根拠による root・子孫・独立会話の解決を検索と原文取得で共有する。子 REF からの探索範囲は [実装契約](v0.14.0-reference.md#実装契約と利用確認に残る事項)として明示する。親欠落・親後着、Claude Code の同じ session ID の兄弟、別入力の同名 ID、発言番号の安定性を fixture で確認する。

- [x] `show` を会話の REF と原文に対応させる

  単一 REF の本人全文、子だけの参照、子孫を会話別に含める指定と JSON 出力を実装する。未知日時・親不明も含め、保存した原文と発言番号へ戻れることを確認する。

- [x] `show` に発言フィルタ・表示範囲と複数会話の一括取得を加える

  [原文取得の方針](v0.14.0-reference.md#原文取得)に従い、複数 REF、role・発言番号・指定日の実発言による絞り込み、limit / offset、一行表示を実装する。元の番号を保ち、フィルタ後に元の順序でページ化してから、各発言の最初の一行だけに短縮する。旧 `outline`・`--summary`・`--turn` の独立入口は統合し、互換性維持を要件にしない。子孫指定との重複・順序、日時境界、Daily Note 用の複数会話取得を確認する。

#### 5. セッションのまとまりを軸にした検索

- [x] 共通照合器とセッション内の全一致検索を実装する

  [パターンと全一致](v0.14.0-reference.md#パターンと全一致)の契約に従い、Go `regexp` の一般的な挙動を具体化して、複数 `-e`・`-F`・まとまりの AND と `search --session REF` を実装する。一致した会話 REF・発言番号・箇所番号・前後の原文を返し、明示した場合だけ箇所単位でページ化する。無効式・空一致、複数箇所、親子をまたぐ AND、`show` との整合を fixture で確認する。

- [x] `search` をまとまりの一覧と総数・ページ化へ切り替える

  [一覧と集約](v0.14.0-reference.md#集約総数ページ化)に従い、入力・source・project の候補条件を先に適用し、共通照合器で判定してまとまり数とページを返す。本文抜粋は一覧に混ぜない。0 件・末尾超過、総数と項目の一貫性、同名 ID、実データでの照合時間を確認する。JSON・TSV の具体案を試用可能な形にし、ユーザーによる最終確認は第 7 段階で行う。

- [x] 一覧に活動日と取り込み日時の絞り込みを加える

  [一覧の日時条件](v0.14.0-reference.md#一覧の絞り込みと日時)に従い、`active` / `started` / `last` / `overlap`、dayBoundary、`--imported-since` を実装する。親子孫全体の既知日時の最小・最大を使い、未知日時、子だけに条件が一致する場合、AND、日付と日時の上限、filter 後の総数を fixture で確認する。

#### 6. CLI の統合と通しの検証

- [x] 一覧検索と原文取得へ CLI の入口を統合する

  `sessions` を `search` 一覧へ統合し、期間で探した複数会話を `show REF...` で取得する。検索 JSON から REF を選んで重複排除し、一回のシェル呼び出しで渡す usage・README・skill の例を揃える。

- [x] 二つの利用場面を取り込みから原文取得まで通して確認する

  複数入力の代表 fixture で、単語検索 → 一覧 → 全一致 → 原文と、指定日の一覧 → 複数会話の実発言取得を通す。親子孫、同名 ID、未知日時・親欠落、REF・発言番号・JSON の整合を検証する。ここは実装側の検証であり、ユーザーの仕様確認は第 7 段階で行う。

#### 7. 実使用とユーザーによる仕様の最終確認

- [x] 実装した利用仕様を試用し、ユーザーと確認する

  第 1〜6 段階では、合意した大枠の範囲で担当エージェントが詳細を決め、決定と理由・入出力例を残して実装を進める。フラグや表示形式の確認を理由にユーザー待ちで止めない。fixture・回帰検証・必須 gate は省略しない。

  サブエージェントに設定作成 → 一覧検索 → ページ送り → 一致箇所 → 原文取得を実際に使わせる。設定の作成・指定とエラー案内、子 REF の探索範囲、日時と AND の判定範囲、発言者フィルタ・limit / offset・一行表示、JSON・TSV の項目とページ情報・既定件数・並び順を確認対象にする。[実装契約](v0.14.0-reference.md#実装契約と利用確認に残る事項)を参照する。

  決めた仕様、具体的な入出力例、サブエージェントの利用結果をユーザーへまとめて提示し、確認を得る。最終確認で決まった変更は即時実装せず、以下の変更タスクとして Backlog に追加する。各変更タスクの実施時に実装・正本・usage・README・skill を同期し、影響する検証と必須 gate を再実行する。完了済みのバージョングループはリリースルールに従い、CHANGELOG への反映と同じコミットで整理する。

#### 8. 利用仕様の確認で決まった変更

- [x] 設定省略を default 指定として扱い、設定生成時に既存 DB も拒否する

  SPEC-01・EX-01 のユーザー確認で決まった変更。DB 操作の `--config` 省略を `--config default` と同じ扱いにする。`import` も省略で default を使い、設定が存在しなければエラーと設定生成案内を返す。設定欠落時の自動生成は行わない。

  `config init` の名前省略は `default` 指定と同じ扱いにする。生成先の設定と、その設定が参照する DB の両方が存在しない場合に設定を生成し、いずれかが存在する場合はエラーとする。名前ごとの既定 DB と明示 `--db` に同じ存在判定を適用し、既存ファイルを変更しない。

  省略と明示 default の同等性、設定欠落、設定のみ既存・DB のみ既存・両方既存での拒否と既存内容保持を確認する。正本・usage・README 両言語・skill を新仕様に同期し、影響する検証と必須 gate が通れば完了とする。

- [x] search 一覧の既定件数を全件にする

  SPEC-11 のユーザー確認で決まった変更。session 指定なしの `search` は、`--limit` を省略した場合に条件に合うまとまりを全件返す。明示 `--limit N` がある場合だけ N 件に制限し、`--offset` は順序付けた結果から指定件数を飛ばす。`--limit 0` は従来どおり項目 0 件とし、総数を保持する。検索条件・集約・並び順は変えない。

  JSON・TSV のページ情報で、件数上限を省略した状態を表現する。全件取得・offset のみ・明示 limit・limit 0・0 件・末尾超過で総数と返却項目が一致することを確認する。正本・usage・README 両言語・skill の既定 20 件の説明と例を同期し、影響する検証と必須 gate が通れば完了とする。

#### 9. レビューで判明した修正と性能改善

`43597927c1443206b64ea7f111214e757bf27423..66c28c76f85534652e0221d465b810129fe7b2c3` の Fresh Review と Finding Verification で、以下の8件を `confirmed`・`actionable` と判定した。FR-001〜006 は CLI で動的再現済み、FR-007・008 は静的な処理量の確認のみ。各タスクは記載した再現条件と完了条件を基準に実装・検証し、共通の必須 gate を通す。

- [x] Claude の本文不変時に取り込み日時を保持する

  FR-001。本文を1件以上保存した Claude 会話へ空行だけを追記して通常 `import` すると、本文・番号が同一でも `imported_at` が更新されることを再現した。不変判定の期待値は `Provenance` が空、取得値は `source_record` であり、構造体全体の比較が不一致となって本人の削除・再保存へ進む。`search --imported-since` に不変会話が混ざる。

  空行追記後も本文・番号・取り込み日時を保持し、取り込み日時の絞り込み結果が変わらないことを確認すれば完了。tool-only / progress の全種類は個別再現しておらず、本文0件は別分岐で本件の対象外。入口は [不変判定と本人置換](../internal/core/import_claude.go) の `claudeOwnerUnchanged`、[本文取得](../internal/core/db_messages_summary.go) の `GetIdentityMessages`。日時保持の契約は [v0.14.0契約](../docs/specs/v0.14.0-contract.md)「原文・順序・日時」と [JSONL schema](../docs/specs/jsonl-schema.md) を参照する。

- [x] Codex の本文なし編集を通常取り込みへ反映する

  FR-002。同本人の a / b rollout を保存し、a を正常な metadata-only ファイルへ編集して b を不変のまま通常 `import` すると、a の旧本文と a=1 / b=2 の番号が残った。同じ最終ファイル集合の `--full` は b=1 だけを保存する。本文なしの a が本人 group の state 集合から外れ、現在残る b の hash だけで不変と判定するため、`show` / `search` が古い内容を返す。

  metadata-only 編集後の通常取り込みと `--full` で本文列・番号が一致することを確認すれば完了。malformed-only 編集の保持方針は対象外。物理削除・入力外移動は同じ skip 経路を静的確認しただけで、独立した契約は未確認。入口は [group構築](../internal/ingest/codex/group.go) の `BuildGroups` と [通常・full取り込み](../internal/core/import_codex.go)。編集再構築と通常 / full 一致の契約は [制約](../docs/rules/constraints.md)「SQLとデータの意味」、[JSONL schema](../docs/specs/jsonl-schema.md)、[v0.14.0契約](../docs/specs/v0.14.0-contract.md) を参照する。

- [x] Codex の循環した親情報でも本人本文を保持する

  FR-003。正常本文を持つ A / B が `parent_thread_id` で互いを指す入力を通常 `import` すると、循環保存エラーで exit 1 となり A 本文だけが残り B 本文が欠落した。続く `--full` も exit 1 となり、入力全体の置換を rollback して A だけの状態に戻る。親と本文を同じ保存経路に置くため、循環辺を未確定として扱う関係解決器へ本人本文が届かない。

  通常取り込みと `--full` の両方で A / B の正常本文を参照でき、循環辺だけが未確定として診断され、確定辺のみを使うことを確認すれば完了。異常 metadata の発生頻度は未確認で、専用 `migrate` の厳格失敗契約は対象外。入口は [親の保存](../internal/core/db_write.go) の `upsertSession`、[本文を含むtransaction](../internal/core/import_codex.go)、[関係解決](../internal/core/session_relations.go) の `buildSessionRelations`。契約は [v0.14.0契約](../docs/specs/v0.14.0-contract.md)「関係とまとまり」を参照する。

- [x] 同時 import による古い snapshot の上書きを防ぐ

  FR-004。Codex / Claude の各 CLI で、先発 A をファイル全文読取後の Git resolver 内で待機させ、追記済みファイルを後発 B が保存してから A を再開すると、両実行 exit 0 のまま新本文が消えた。cursor は Codex で 352→226、Claude で 368→184、`imported_at` も1秒退行した。snapshot 解析後に期待 cursor を取得するため、A が B の新 cursor を期待値として受け入れ、transaction 内の再比較を通って古い本文を保存する。

  両 source でこの実行順を決定的に作り、後発が保存した新本文・cursor・取り込み日時を先発が退行させないことを確認すれば完了。実環境の重複実行頻度は未測定で、ログが残れば次回取り込みで復旧できる。`--full` は別経路で再現対象外。入口は [Codex取り込み](../internal/core/import_codex.go) と [Claude取り込み](../internal/core/import_claude.go) の snapshot 作成・旧 state 取得・transaction 内再比較、および全文を読む [Codex group構築](../internal/ingest/codex/group.go) / [Claude snapshot構築](../internal/ingest/claudecode/snapshot.go)。

- [x] migrate 再実行前に欠落した rollout の本文を保持する

  FR-005。空の有効 legacy-v013 snapshot と同本人 a / b rollout で初回 `migrate` を行い、b を削除して同じ snapshot / config で再実行すると、exit 0・`copy_performed=false`・`groups_replaced=1` のまま b の正常本文が消え、b の旧 cursor だけが残った。当回の発見集合と再走査だけを比較し、前回保存した rollout 集合を照合しないため、a だけの本人全文で置換する。

  再実行の開始前に一部 rollout が欠落した場合、当該 group を成功置換せず、既存正常本文・旧行・cursor を保持することを確認すれば完了。全 rollout が消えて group 自体がない場合は本件の対象外。動的再現は旧本文0件の snapshot であり、初回削除済み legacy コピーを receipt により復元できない追加影響は静的確認のみ。入口は [group検査・置換](../internal/core/migrate.go) の `replaceMigrationGroup` と [receiptによるコピー省略](../internal/core/migrate_snapshot.go)。保持契約は [移行契約](../docs/specs/v0.14.0-migration.md) の rollout 欠落時の失敗条件を参照する。

- [x] Claude snapshot の同一 cwd で Git 解決を再利用する

  FR-006。通常 repository の同じ cwd を持つ20 user record を CLI で取り込み、PATH の wrapper で数えると Git 起動は初回40回、不変再取り込みも40回だった。後者は1 file skip でも、hash 判定より前に全 record で resolver を呼ぶ。旧 adapter の cwd cache を使わず、通常 repository で record 数 R に対して2R回の同期外部 process 待ちが生じる。

  同じ cwd の record 数を増やしても Git 解決の起動数が record 数に比例せず、repository / branch の結果と不変取り込みの skip が維持されることを確認すれば完了。異なる cwd の結果を混同しないことも確認する。wall-clock 性能と実ログでの頻度は未測定。空 cwd と `/.claude/worktrees/` は Git 不要で、linked worktree は通常より起動数が多い場合がある。過去の取り込み中止原因には帰属しない。入口は [snapshot構築](../internal/ingest/claudecode/snapshot.go)、[hash判定](../internal/core/import_claude.go)、[Git resolver](../internal/core/repo_path.go)。再利用の既存例は [adapter](../internal/ingest/claudecode/adapter.go) の cwd cache を参照する。

- [x] migrate の group ごとの無関係な legacy 全件読取を減らす

  FR-007。多数の Codex group と legacy 発言を持つ snapshot の `migrate` では、UUID→group evidence を事前構築済みでも、各 group の write transaction 内で `source='codex'` の残存 legacy 行をすべて Scan し、Go 側で対象を選ぶ。G group の各回に残存 L_i 行を読み、置換不能な行が残れば G×L の反復が生じることを静的確認した。[実ログの規模](../docs/specs/v0.14.0-log-evidence.md) は旧 DB 14,783会話・196,787発言を記録している。

  当該 group に無関係な全行を毎回 Go へ移送する処理を減らし、大きい fixture で読取量と所要時間を比較して改善を確認すれば完了。旧行の限定削除・競合証拠の保持・失敗時 rollback の契約も既存 fixture で維持する。実際の G・L_i・削除分布・経過時間・SQLite cache 効果は未測定で、G×L/2 は均等削除時の近似にすぎない。inventory 再検査の安全保証と費用は本件に含めない。入口は [migration](../internal/core/migrate.go) の evidence 構築と `replaceMigrationGroup` の全行 query、[UUID UNIQUE制約](../internal/core/db_schema.go)。削除・保持契約は [移行契約](../docs/specs/v0.14.0-migration.md) を参照する。

- [x] 複数 REF の show で namespace 集計と関係構築を共有する

  FR-008。help にある `search` の `.items[].members[]` を複数 REF として `show` へ渡す経路では、同じ Claude / Codex namespace の全 session・全 message 集約と graph 構築を REF ごとに繰り返す。子孫指定なしでも発言フィルタと重複除外より前に行うため、同 namespace の K REF で集約・構築が K 回になることを静的確認した。

  同 namespace の複数 REF で広範な集約・関係構築を共有し、REF 数と履歴量を増やした fixture で処理回数と所要時間を比較して改善を確認すれば完了。本人 / 子孫、重複・順序、発言フィルタ・番号・ページ化・JSON の結果を維持し、別 namespace を混同しないことも確認する。実測時間・query plan・cache 効果は未測定で、`OCTET_LENGTH` 集約を本文全 bytes の逐次読取とは断定しない。Cursor / legacy は早期 return で対象外。入口は [showのREF loop](../cmd/somniloq/show.go)、[ResolveSession](../internal/core/session_relations.go)、[message JOINと集約](../internal/core/db_sessions_projects.go)。出力契約は [v0.14.0契約](../docs/specs/v0.14.0-contract.md) の原文取得を参照する。
