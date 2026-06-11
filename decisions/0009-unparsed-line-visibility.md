# ADR 0009: parse 失敗行は黙殺せずカウントして import 結果に表示する

## Status

Accepted

## Context

両 adapter とも `ParseRecord` / Normalize 失敗行を `continue` で黙殺しており、件数も残らなかった。壊れ JSON と「仕様として無視する record type」の区別がなく、「なぜこのメッセージが DB にないか」を追う手段がなかった。backlog はこれを「可視化するか、黙殺を仕様として明文化するか決める」タスクとして積んでいた。

## Considered Options

- **A: 解釈失敗行をカウントして import 出力に表示する**: `FileHandler.HandleLine` の戻り値を `LineOutcome`（Ignored / WroteBody / Unparsed）にし、`ProcessJSONL` が Unparsed をカウント、`ImportResult.UnparsedLines` 経由で CLI のサマリ行に出す。
- **B: 黙殺を仕様として `rules/scope.md` に明文化する**: コードは変えずドキュメントだけ置く。

## Decision

We will count lines that fail to parse or normalize and report them in the import summary (Option A). 「DB にない理由を追えない」は Knowledge 運用上の現在の困りごとであり、文書化（規約レベルの最弱の防御）よりカウント（実行時の可視化）の方が強い仕組みで問題を防ぐ。

カウントの線引き:

- **カウントする（LineUnparsed）**: JSON として壊れている行、payload / message envelope / content の構造が解釈できない行。
- **カウントしない（LineIgnored）**: 空行、source が仕様として無視する record type（claude-code の summary 等、codex の非 message response_item / event_msg）、codex で session_meta 出現前に到着した message。これらは「正常な入力の意図的な無視」であり、混ぜると正常ファイルでも常に大きな数が出てシグナルが消える。

表示は既存サマリ行の拡張（`Imported N files (... , N unparsed lines)`）で常時表示。exit code には影響させない（壊れ行はファイル内容の問題であり import 自体は成功している。失敗扱いは `FilesFailed` のみ）。

## Consequences

- import 後に「何行解釈できなかったか」が見えるため、メッセージ欠落の一次切り分けができる。
- meta-only ファイル（offset が進まない）の unparsed 行は import のたびに再報告される。「その実行で読んで解釈できなかった行数」という一貫した意味になる。
- どの行が落ちたか（行番号・内容）までは出ない。必要になったら別タスクで詳細化する。
- `Adapter.ProcessFile` の戻り値が `ProcessResult` struct になり、走査エラー非致命化（backlog 次タスク）等の今後の契約変更を受けやすくなる。
