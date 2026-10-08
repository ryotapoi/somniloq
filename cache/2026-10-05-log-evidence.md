---
observed_at: 2026-10-05
compiled_at: 2026-10-08
compiled_from_commit: a2f63a7
source_record_commits:
  - d9a6ced7f3dba370952fe1b04901230be3624bdc
  - 50fed22389c1eedf852b472c7f7e86185458ce8b
---

# 元ログの時点付き観測

原文の記録 commit は観測本文の履歴であり、計測時の正確な実行 binary commit を証明しない。再編纂する場合は `git ls-tree -r --name-only a2f63a7` から `v0.14.0-log-evidence.md` と `jsonl-schema.md` の一致名を探し、各 `git show a2f63a7:<見つけたpath>` を読む。先行 Cursor 件数と Claude 版差は後者から抽出する。新たな観測では対象 root の各ファイル size を走査開始時に固定し、その byte 範囲を read-only で解析して集計値だけ残す。元本文・実 ID・実 path は記録しない。

## 方法と範囲

2026-10-05、標準 root の Codex sessions、Claude projects、Cursor projects を read-only で構造走査した。各走査の開始時に対象ファイルの size を固定し、その byte 範囲だけを読む。個人本文・実 ID・実 path は保存も fixture へ転記もしない。本文の比較はメモリ内で行い、集計値のみを残す。稼働中のログなので別走査時の inventory／records は変わり得る。元ログの完全 snapshot や全形式保証ではない。

一時スクリプト・集計は調査時の作業物であり、この cache の再導出に依存しない。2026-10-04 の既存調査は先頭制限で Codex 親子30組、Claude 全構造、Cursor 全構造を確認済みだった。今回の追加は最新 Codex 160 rollout の全文と孫の接続、および Claude の同一物理親内の照合・重複 result・未確定例の分離を対象とした。既調査の全量を単にやり直したものとして完了理由にしない。

## 観測根拠

| source | 追加で確認した事実 | 限界 |
| --- | --- | --- |
| Codex | 最新160 rollout / 42,757 records を全文走査。子103、サンプル内孫69、境界あり9、複数metadata4。本文2,219 records は全件ordinalあり。境界未満4 records はサンプル内親のpayloadと全件一致。境界あり9例のうち本人最初の本文がassistantの例は4。最初の metadata の `source.subagent.thread_spawn.parent_thread_id` で直接親を取得でき、サンプル内に子と孫を含む。埋込 metadata と明示境界を含む例もある。本文 ordinal は top-level にあり、境界は最初の metadata payload の `subagent_history_start_ordinal` にある | 最新サンプルは ordinal のある新形式に偏る。境界／ordinal 無の旧形式は既調査の各10組を根拠とし、未知形式にまで除外規則を推測しない |
| Claude Code | 1,768 files / 212,287 records、子1,175。同一物理親内 Agent/Task call と `tool_result.tool_use_id`、record-level `toolUseResult.agentId`、子の path/agentId を照合すると1,032子の親を確定。直接親は root が551、子が481。1,031子で最初の user と呼出 prompt が一致。7候補は同一親/promptの反復 result で、重複をまとめた後に複数親候補は0 | 残る143子は直接親未確定。prompt 一致だけで親を決めない。`fork-context-ref` 4件の参照本文を取得できたとはしない。root 所属は path、直接親は call/result の別根拠 |
| Cursor Agent | 708 files / 11,280 records。top-level timestamp/cwd/parent/parentId/sessionId は全件なし | root・path から session ID は読めるが日時・project・直接親を推測できない。キーなしの観測は全バージョン保証ではない |

Codex の集計値は走査が終了した artifact の値を採用する。payload 全体を正規化 JSON の hash で比較した結果も同 artifact に残す。本文そのものや hash を公開しない。最新サンプル内に直接親がない場合は一致の分母へ含めず、親子本文の一致なしと解釈しない。

## 2026-10-04 の先行調査

以下は v0.14.0 実装前の調査であり、当時の旧 schema・parser を対象とした観測である。旧 DB の物理的出自と本人帰属を分ける根拠として残す。2026-10-05 の追加全文走査とは範囲が異なり、先頭制限で確認した件数を全量の所属保証として扱わない。

| 対象 | 観測と範囲 | 限界 |
| --- | --- | --- |
| Codex 旧 DB と残存ファイル | 固定バックアップには14,783会話・196,787発言。取り込み済みpath15,957件中15,956ファイルが現存。各ファイルの先頭256 KiBだけのUUID照合で14,759会話・70,581発言に対応する物理行を確認した | 旧schemaには入力IDと会話・ファイルの直接リンクがなく、確認件数は対応の下限。会話全体の入力所属が確定した件数ではない |
| Codex 複数metadataと旧帰属 | 同じ先頭走査で1,181ファイルに複数の有効なsession_meta ID、46ファイルに複数の保存済みsession IDと一致するmessage UUIDがあった。当時のparserはmetadataごとにactive identityを更新していた | 複数IDは親文脈metadataを含み得る。UUID一致は旧行の物理的出自の証明であり、旧session IDによる本人帰属の証明ではない |
| Codex 境界あり親子10組 | 子は5 MiB以下、親は先頭8 MiBまでを比較。全組で最初のmetadataは子ID、次の埋込metadataは直接親ID。境界未満のuser/assistant47発言は親のpayload ID・payloadと本文hashに全件一致し、境界以降の本人24発言は不一致。本人先頭は全組assistant | 先頭userを本人境界にできない。親の読み取り制限があり、この観測は全ログ形式の保証ではない |
| Codex 境界なし親子20組 | 上と同じ制限で、境界もordinalもない10組（本人79発言）とordinalだけある10組（本人44発言）を比較。親payloadと一致する本文はなく、全ファイルのmetadataは一つ | 観測例であり旧形式全体の保証ではない。境界のない入力へ本文除外を推測で適用しない |

Claudeの構造走査では、子1,175のagentIdは全件ファイル名と一致し、sessionIdは所属rootのディレクトリ名と一致した。構造化toolUseResultのagentId一致は1,112子（root側627・子側485）だったが、これは同一物理親内のcall/resultまで厳密に確定した1,032子とは異なる前段の集計である。厳密な直接親の集計・prompt一致・未確定143子・fork-context-refの限界は上の追加観測を参照する。

Codexの明示境界の意味は、当時参照した公式sourceの[値の定義](https://github.com/openai/codex/blob/main/codex-rs/thread-store/src/types.rs#L2371-L2380)と[履歴projection](https://github.com/openai/codex/blob/main/codex-rs/thread-store/src/local/thread_history_materialization.rs#L1404-L1414)でも確認した。境界は本人のprojected history開始ordinalであり、境界未満を継承履歴として扱う根拠となった。リンクは調査時点の参照先である。

同日のGo 1.27.1で標準regexpの大小文字、Unicodeの(?i)、非重複UTF8 byte区間、ゼロ幅、QuoteMetaの挙動を確認した。[regexp](https://pkg.go.dev/regexp)と[構文](https://pkg.go.dev/regexp/syntax)を参照した実験であり、現行の検索仕様・期待値はコードと query fixture を参照する。SQLiteの`:memory:` DBにdeterministic scalar functionを登録し、bindしたpatternを渡す実験も成功した。これは登録・受け渡しの成立確認であり、実データの照合性能を示さない。性能の取得範囲は[検索計測](2026-10-05-search-measurements.md)を参照する。


## 日付が確定していない先行 Cursor 観測

Cursor Agent `2026.09.10-fd3934a` のローカルログ 607 files / 9,259 records では、transcript path、directory と basename の一致、`role=user|assistant`、text / tool_use、`turn_ended` を確認した。prefix を保つ末尾追記は controlled follow-up で確認した。観測日を原文から確定できないため、2026-10-05 の 708 files / 11,280 records と合算しない。独立 timestamp、cwd、repository、version、title、usage、parent は観測されず、slug の可逆性も保証できない。

Claude Code の版 v2.1.37〜v2.1.86 の既存観測では `isMeta`、`slug`、`permissionMode`、`todos`、`thinkingMetadata`、`planContent`、`imagePasteIds`、`promptId`、`toolUseResult`、`entrypoint` に増減がある。この版別一覧の観測日は確定できず、全版の固定 schema を意味しない。
