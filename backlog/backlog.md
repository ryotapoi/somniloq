# Backlog

## 運用ルール

- Backlog には、今後着手でき、完了を判定できる作業を `- [ ]` 形式のタスクとして置く。
- 完了したタスクは `- [x]` にし、版を切る変更（バージョンを上げてタグを打つ変更）で消す。取りやめたタスクは消す。
- 対応要否を決める調査もタスクにできる。観察事実だけを残さず、判断したい問いと、判断をもって完了とすることを明示する。
- 粗いメモで起票してよいが、着手前に目的と受け入れ条件を読める粒度へ詰める。
- タスクの行には短い作業名を書き、説明は空行を挟んだ字下げ段落に分ける。
- バージョン番号を付ける場合は SemVer に従う。
- タスクは `###` 見出しでグループに分ける。バージョンが確定したグループだけ、`### v0.1.0 名前` のように見出しへ番号を付ける。
- グループが大きい場合は、`####` 見出しでさらに分割してよい。

## タスク

### v0.14.4

#### コード整理とレビュー

- [x] simplify-code を実行する

- [x] simplify-tests を実行する

- [x] コードレビューを実行する

#### 検証とリリース準備

- [x] 検索成功 fixture と代表一覧 oracle を一致させる

  `internal/ingest/testdata/v0.14.0-query` の README に成功例の6ファイルだけを一時入力へコピーして実行する手順を置き、listExample から成功例に含まれない unknown-owner を除いて5本人へ揃えた。build CLI で通常 import が成功し、host 依存の入力キー・root と取り込み時点を除いた一覧 envelope が期待値と一致することを確認した。

- [x] 通知生成と照合の文字コード依存をなくす

  `scripts/update-third-party-notices.py` のファイル読取・書込と subprocess 出力の解読に UTF-8 を明示した。Go CLI や DB は変更しない。Python の既定文字コードが ASCII の環境で旧版の読取失敗を再現し、修正版の生成内容が既存の UTF-8 通知の bytes と一致することを確認した。非 UTF-8 形式の取込対応や新しい保存方式は追加しない。

#### 保存データの保護

- [x] Cursor の full 再構築失敗時に旧履歴を保護する

  Cursor full を入力単位の transaction で削除・再保存し、走査・読取・保存・最終 commit が失敗した入力の旧本文・session・cursor を保持する。複数ファイルの後半失敗、同サイズ変更、欠落ログ、本文なしファイル、空入力、対象外入力、full 後の追記を回帰テストで確認した。

- [x] Claude の古い full snapshot による上書きを防ぐ

  DB の canonical path に対応する `.lock` sidecar の OS 排他を、通常・full・source を問わず import の DB open 前と migrate の移行元検査前に取得し、終了まで保持する。後発は待機せず終了1。実プロセス間の競合拒否、別 DB、正常終了・open 失敗・強制終了後の再取得、canonical alias、および Claude full の読取中の後発拒否と解放後の本文・metadata・cursor の追記反映を回帰テストで確認した。

- [x] Codex の古い full 走査結果による削除を防ぐ

  共通の書込み排他で Codex full のファイル一覧取得前から終了まで保護する。実プロセスの先発 Codex full に対し、全 source の通常・full import と migrate が DB open 前に busy・終了1となることを確認した。一覧確定後に新 rollout を追加し、後発の拒否、full 完了・解放後の再 import による新 rollout の本文と cursor（サイズ・offset・hash）の保存を回帰テストで確認した。

- [x] 古い移行 snapshot による上書きを防ぐ

  共通の書込み排他を移行先 DB の変更と元ログの走査・読取より前から migrate の終了まで保持する。同一 snapshot・移行先への再実行でも実プロセスの後発を拒否し、本文読取後の追記に対する後発 import 相当の lock 取得と migrate の busy、解放後の import による新本文・cursor（サイズ・offset・hash）の反映を回帰テストで確認した。元 snapshot の bytes・sidecar 不変と、再実行時のコピー非重複も維持する。

- [x] 特殊文字を含む DB パスで指定先と権限を守る

  DB 名に `?` を含むと未 escape の DSN が別ファイルを選び、明示した 0600 作成や移行先の identity 検査を迂回する。対応方針は、DB パスを SQLite の file URI に標準の URL エンコードで組み込み、ファイル名の文字を接続オプションとして解釈させない方式で確定する。`internal/core/db.go` と `internal/core/migrate_snapshot.go` の接続文字列生成を整合させ、import・読取・移行が同じ指定ファイルを扱い、新規 DB の 0600 権限を維持することを確認する。

- [x] Codex の空・識別不能な読取結果で旧履歴を消さない

  走査で認識した会話 ID が読取時に空になった場合を既存の一致検証で拒否し、full の transaction 全体を rollback する。空ログ・ID のない非空ログと、同じ会話の複数 rollout の部分喪失で、旧本文・session・全 cursor の保持と失敗報告を回帰テストで確認した。会話 ID を含む有効な短縮ログは現在の本文・session・cursor（サイズ・offset・hash）へ置換する。一般的なログ書換え追従の強化は含めない。

- [x] 初回 DB 初期化で一貫した schema 判定を行う

  import・migrate の共通書込み排他を DB のオープン（schema 検査・初期化）より前に取得し、終了まで保持する。未作成 DB の先発を open 境界で停止した実プロセステストで、全 source の通常・full import と migrate の後発が DB open 前に busy・終了1となり、先発の正常初期化と解放後の import が成功することを確認した。初期化専用の排他は追加しない。

#### CLI と設定

- [x] config init で設定先と DB 先の重なりを拒否する

  `cmd/somniloq/config_init.go` のコマンド検証で相対指定と親 symlink を解決し、DB 先が設定自身またはその親ディレクトリ（祖先を含む）になる指定を生成前に終了2で拒否する。未存在の設定親も生成しないこと、類似 prefix の別パスを受理すること、正常指定では読込可能な設定だけを生成して DB を作成しないことを回帰テストで確認した。help と README 両言語も同期した。

- [x] search と show の不正な日時 offset を拒否する

  `--since`・`--until` と search の `--imported-since` で原文の RFC3339 offset を検査し、時24・分60を補正受理せず DB open 前の引数エラー終了2で拒否する。projects と同じ境界、正常 offset・小数秒・等価 instant の選択と下限包含・日時上限排他を回帰テストで確認した。help と README 両言語も同期した。

- [x] projects に表示した full-path alias で検索できるようにする

  完全一致で展開した alias group 内の full path を保存 repository path 全体への literal equality として候補選択に渡す。canonical 名・旧 full path のどちらからも pattern なし・あり・指定 REF の search で選べること、projects の集約件数と保存値の不変、別ディレクトリの同 basename を巻き込まないことを確認した。通常の basename substring と非 alias 入力の意味は維持する。

- [x] full path と basename が交差する alias の曖昧さを拒否する

  canonical 名と旧名の完全一致重複拒否を維持し、full path とその basename が別 group に属する設定も読込時に拒否する。同じ group 内の交差、および別 group の異なる full path が同 basename を持つだけの設定は受理する。canonical・旧名の配置と group の走査順によらない拒否、受理した設定の projects 集約名と件数を確認した。

#### 取り込みの正確性

- [x] 新しい Claude sibling の親競合を古い snapshot で戻さない

  通常・full import の snapshot 読取後に競合する sibling を追加し、先発処理中の後発操作が共通排他で拒否されることを確認した。排他解放後の再 import は直接親を消し、root grouping を保持する。root の子孫から競合 child と grandchild を除外し、child の確定 grandchild は選択できること、変更なし再 import でも親が戻らないことを確認した。

- [x] Codex の通常 import で欠落ログ由来の旧履歴を保持する

  通常 import は欠落 rollout の保存済み本文・元ブロック・timestamp・由来・membership と既知の親を保持し、残存 rollout の新規本文と物理順で統合する。payload の重複排除と本文番号を再構成し、full 成功時だけ選択入力を現在ログから作り直す。先頭・後続 rollout の削除、追記、無変更再実行、context / unresolved、保存失敗時の本文・cursor 保持、full と対象外入力の保持を回帰テストで確認した。

- [x] Codex の本文なし rollout も会話情報の取込判定に含める

  本文あり会話に有効な本文なし rollout が含まれる場合、保存済み会話 metadata も通常 import の skip 判定で比較する。先行パスへの新規追加による cwd・branch・version の反映、無変更再実行、full との一致、本文・番号・identity・REF の保持、本文なし cursor の非作成を回帰テストで確認した。

#### repository の解決

- [x] Git パス内の改行を repository 名として保持する

  Git の単独パス出力は終端 LF 1文字だけを除去し、git-dir と common-dir は個別に照会する。パス内・末尾の CR・LF を保持し、本体と linked worktree の同一 repo_path、Claude Code / Codex の保存・project 集約、通常 import の旧値保持と full による再解決を回帰テストで確認した。

- [x] ルート直下の Claude worktree を repository に解決する

  `/.claude/worktrees/…` とその配下を Git 照会前に `/` に解決する。未知 repository との集約の分離、project `/` による検索、通常 worktree の解決を確認した。保存済みの旧空値は変更なし import では保持され、元ログを選んだ `--full --yes` で再解決できる。

#### 検索と出力の整理

- [x] projects の TSV にも既存の端末制御文字対策を適用する

  `projects` の TSV を `search`・`show` と同じ共通文字変換へそろえ、ESC は `\x1b`、tab・改行・backslash も可逆な TSV escape で返す。help・公開形式・README 両言語を同期した。実 JSONL の取り込みから出力までで raw 制御文字の不在と一回の復元、保存値・JSON の不変を確認し、alias 合算・件数・short 表示と別 repository の分離を回帰テストで確認した。

- [x] TSV のログ由来の端末制御文字を無害化する

  詳細 search・show の本文にログ由来の ESC 等があると、既定 TSV がそのまま出力し対応端末で画面消去やカーソル移動を起こす。対応方針は、TSV を維持し、既存の共通文字変換で制御文字を可逆な見える表記へ変換する方式で確定する。ESC の `\x1b` 等の表記と復元規則を公開形式にそろえ、端末に制御文字をそのまま出さず、既存の TSV escape と原文保存・JSON の契約を維持することを確認する。

- [x] 詳細検索の重複した関係解決を減らす

  成功する search --session が同じ read snapshot 内で REF と namespace graph を二度解決する。一度求めた結果を同じ検索内で再利用する局所改善の方向で対応する。不正 REF・会話不存在・I/O エラーの終了コード、診断、関係判定、snapshot の一貫性を維持し、DB 読出し・割当て量・検索時間の変化を確認する。操作をまたぐ長期 cache は追加しない。局所改善に収まらず保守負担との一長一短が生じる場合は、improve-performance による計測と採否判断の候補として扱う。

- [x] 期間だけの非 active 検索で不要な本文取得を減らす

  pattern のない期間検索を非 active の time-mode で実行しても、選択に使わない候補全本文を読込み保持する。既存の本文取得条件を整理し、この場合は取得を省く局所改善の方向で対応する。期間・件数・page の選択結果と active・本文照合の動作を維持し、本文量を変えた前後計測で読出し量・ピークメモリ・検索時間を確認する。一長一短が生じる方式が必要な場合は、improve-performance による計測と採否判断の候補として扱う。

- [x] TSV escape 表の反復構築を減らす

  大量 TSV の各文字列で固定 escape 表を作り直し、escape 不要の REF にも累積割当てと GC 負担が生じる。TSV の escape 結果を維持して反復構築を減らし、show・詳細 search の大量出力を前後計測して割当て量と出力時間を確認する。

#### 不要な処理の除去

- [x] Claude の不要な旧会話検索を除去する

  現在のログから会話 ID を読んだ後も、ファイルごとに保存済み本文の origin_path から旧会話を検索し、旧 ID を会話の置換対象へ加えている。ID が変わっても新しい ID の会話として受理し、旧 ID は過去の履歴として残す方針で確定する。このため `internal/core/import_claude.go` の旧会話検索と旧 ID の置換対象追加を除去する。現在のログの ID は従来どおり記録から読み、同じ ID の通常追記・重複処理・親子関係を維持すること、別 ID を追加しても旧 ID を残すこと、full は選択入力を現在のログから作り直すことを確認する。対応表・索引・schema は追加しない。

- [x] 不要な旧 Claude importer と専用検証を除去する

  通常の Claude import が通らない旧 importer に別の保存規則と専用テストが残り、一部 concurrency wrapper も旧経路を実行する。二重管理をなくすため、旧 per-file 保存処理とその専用の付属処理・検証を除去する方針で確定する。必要な検証は現行 snapshot 経路へ合わせ、通常経路の保存・並行処理の保証を維持する。共通 adapter 契約を現在の呼出しに合わせて整理し、Cursor の現役 per-file 処理と共有 parser・旧入力形式の対応を維持する。残る concurrency 検証が通常経路を対象とすることを確認する。

- [x] 不要な旧 Codex offset 再開実装を除去する

  Codex の旧 per-file 再開入口と prefix 復元処理・専用検証を除去した。metadata 回復・無効 ID の物理行付き診断・unknown timestamp の検証を owner group 経路へ移し、共有 handler・parser、Cursor の offset 再開と既存の group・移行の保証を維持した。

#### improve-performance で検討するもの

- [x] improve-performance で Claude 差分 import のメモリ・時間の改善価値を検討する

  保存・履歴保持・関係判定と全ファイルの解釈を維持し、raw bytes を各ファイルの診断集計後に保持しない方式を採用した。[32 ファイルの対比較](../cache/2026-10-12-claude-import-memory.md) でピーク RSS を 33–67% 削減し、取込時間の実質的な悪化なしを確認した。tool payload 中心の入力では CPU 時間が約 17–19% 増える代償と、一個の巨大ファイル・normalized records の全履歴保持が残る限界を記録した。

- [x] improve-performance で解析失敗の診断情報の保持量を改善できるか検討する

  全失敗数・最初の失敗・cursor 以後の先頭 5 件だけを逐次集計する方式を採用した。[40 万失敗行の対比較](../cache/2026-10-12-diagnostic-retention.md) で初回取込のピーク RSS を Claude 90%、Codex 76%、取込時間を 26%／46% 削減した。総数・公開診断・新規失敗・未完成末尾・厳密な移行と履歴保持を維持し、正常 Claude 初回取込の約 4 ms 増と未計測の規模を記録した。元ログは削除しない。

- [x] improve-performance で Claude の会話比較のメモリ・時間の改善価値を検討する

  変更された会話だけの record と title・name の証拠を一度集め、会話ごとの全 snapshot record 反復走査を除去した。[対比較](../cache/2026-10-12-claude-owner-comparison.md)では 128 会話中 96 件の追記で比較時間を 54.1→39.4 ms、512 会話中 384 件の追記で 117.9→55.5 ms に短縮した。全体取込の改善は 512 件の初回測定では再現しなかったが、後続の対比較では約 5% の短縮を観測した。ピークメモリは実質同じで比較区間の割当ては約 1% 増えた。本文・metadata・imported_at・履歴保持の回帰確認を通した。

- [ ] improve-performance で show の少量表示の改善価値を検討する

  show --tail・--messages・--limit 等でも、選択会話の全本文と blocks を取得・decode してから絞り込む。improve-performance skill による計測と採否検討のタスクで確定し、方式の採用は先に決めない。保守負担の実質的な増加を避け、メモリ削減だけを理由に明確な速度悪化を受け入れない。絞込み条件を既存の取得に反映する小さい案と、総件数・複数 REF の順序・tail・page を保つ取得変更の案を分けて評価する。絞込み順序・総件数・出力内容を維持し、長い会話の少量出力と通常の全件出力を前後計測して読出し量・ピークメモリ・応答時間を比較する。採用案または見送り理由を根拠付きで判断して検討完了とする。

#### 対応方針・採否が未確定のもの

- [ ] Codex owner 置換の入力全本文走査を減らす

  初回を含む多数 owner の import・移行で、同じ会話 ID の保存を更新する際、対象会話の本文を見つけるために同じ入力の全 message を探索し write transaction の負担が増える。会話の更新自体を取りやめず、既存の識別情報・索引と問い合わせ条件を整合させて反復走査を減らす。通常 import の欠落ログ由来の履歴保持と移行の保存保証を維持し、owner 数・本文数を変えた前後の query plan と取込・移行時間を確認する。

- [ ] 旧 DB から残した会話本文の全体走査を減らす

  migrate が旧形式 DB から保存した legacy 履歴を複数 REF で show すると、選択会話ごとに旧履歴全体の本文を探索する。legacy は既存どおり指定会話を独立して参照し、親子関係の復元・推測や他の会話への展開は追加しない。その参照経路を維持し、対象会話を探す問い合わせの走査を減らす。取得内容と順序を維持し、旧履歴の規模・選択会話数を変えた前後の query plan と取得時間を確認する。

- [ ] 詳細検索の一致ごとの重複文字走査を減らす

  長い一行に多数一致があると、各 span の lineText 計算が同じ文字列を繰り返し走査し、limit もその後に適用される。必須対応とはせず、保守負担に見合う方法で改善できれば対応する。一致・行番号・表示本文・総件数を維持して走査負担を減らし、行長と一致密度を変えた前後計測で検索時間とメモリの変化を確認する。

- [ ] 単一 REF の show で無関係な本文集約を減らす

  単一の小さい会話を show しても、関係解決の初回に namespace 全体の message 件数と byte 数を集約する。性能を悪化させずコードも改善する局所整理に収まる場合は修正タスクとして対応し、そうでなければ improve-performance による計測と採否検討へ切り替える方針で確定する。まず既存の親子関係処理を維持したまま、その取得に不要な本文集約を分離できるか評価する。関係 graph の取得範囲や診断ルールの変更、長期 cache はこの案に含めない。必要な parent・root・診断の関係解決と表示値を維持し、対象会話を固定して無関係な履歴量を変えた前後計測で SQL 負担・応答時間・ピークメモリを確認する。

### 低優先度

- [ ] symlink と親参照を含むパスを実体どおりに解決する

  設定の入力 root・DB パスに symlink 直後の `..` があると、字句正規化が指定と異なる実体を選ぶ。`internal/core/input.go` で絶対・相対パスが指定した実体へ解決され、実体 root の直接指定と入力 identity・重複除外が一致することを確認する。
