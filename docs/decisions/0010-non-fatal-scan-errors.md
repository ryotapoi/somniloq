# ADR 0010: ディレクトリ走査エラーの非致命扱い

## Status

Accepted

## Context

import はファイル処理のエラーを非致命として扱い（失敗ファイルをスキップして
`ImportResult.Errors` に記録し、exit code 1 で続行）、他のファイルの取り込みを
継続する。一方、ディレクトリ走査（`ScanFiles`）のエラーは致命扱いで、読めない
サブディレクトリが 1 つあるだけで import 全体が失敗していた。読めないディレクトリ
は権限の問題などで恒常的に存在しうるため、走査エラーだけ全体を止める非対称は
ユーザーにとって不便だった。

旧 `ScanFiles` の戻り値は `([]File, error)` で、エラーを返すと部分結果を返す
余地がなかった。

## Considered Options

- **A: `ScanFiles` を `([]File, []error)` に変更**: 部分結果と非致命エラー群を
  同時に返す。呼び出し側（core）が errs を `ImportResult.Errors` へ記録して続行する
- **B: `ScanFiles` がエラーを内部で握りつぶしてログに出す**: シグネチャは不変だが、
  エラーが `ImportResult` に乗らず exit code にも反映されない
- **C: callback / visitor 形式に変更**: ファイル発見とエラーを逐次通知する。
  柔軟だが、現状の用途（全件集めてからループ）には過剰

## Decision

We will change `ingest.Adapter.ScanFiles` to return `(files []File, errs []error)`:
partial results plus non-fatal errors.

- 読めないディレクトリはスキップし、`errs` にパス入りのエラーを記録して走査を続ける
- rootDir 自体が存在しない場合は「source 未使用」を意味し、`nil, nil` を返す
  （エラーではない）
- adapter がエラーにパス入りの scan コンテキスト（`scan <path>: ...`）を付け、
  core は `errs` をそのまま `ImportResult.Errors` へ記録して、発見できたファイルの
  取り込みを続行する
- exit code はファイル処理失敗と同じく `len(result.Errors) > 0` で 1 になる

## Consequences

- 読めないディレクトリがあっても残りのファイルが取り込まれ、エラーは
  `ImportResult.Errors` と exit code で可視化される（ファイル単位エラーと対称）
- ファイル処理失敗と走査失敗が同じ `Errors` に混在する。exit code 上は区別不要
  だが、区別が必要になったらエラー型の導入が要る
- `[]error` を返す Go のシグネチャはやや珍しいが、「部分結果 + 複数の非致命
  エラー」という契約を型で表せる（`error` 1 つに畳むと件数とパスの構造が落ちる）
- 新しい source の adapter も同じ契約（スキップ + 記録 + rootDir 不存在は
  `nil, nil`）に従う必要がある（interface doc に明記済み）
