# リリース

## タグと GitHub Release

- タグを打つだけのバージョンと、GitHub Release を作成するバージョンを分ける。タグごとに GitHub Release を作成する必要はない。
- GitHub Release は明示的に指示されたバージョンで作成する。途中のバージョンは CHANGELOG に記録し、GitHub Release は作成しない。

## GitHub Release の手順

依存関係・Go version の変更時は、次の手順で通知を再生成し、差分の
ライセンス原文・著作権表示・追加通知を確認する。Python 3.9 以降を使用する。

```bash
go install github.com/google/go-licenses/v2@v2.0.1
python3 scripts/update-third-party-notices.py
```

`THIRD-PARTY-NOTICES.txt` は `go.mod` の直接・間接依存に加え、
darwin / linux / windows の amd64 / arm64 ビルドの依存通知を
`go-licenses save` で収集する。Go 本体・標準ライブラリ内の vendor、
SQLite の public domain 通知、libc 内の Go・musl の通知、memory の
`LICENSE-MMAP-GO`（Evan Shaw の BSD ライセンス）も原文で補う。
libc 内に埋め込まれた uint128・Windows の BSD 等の通知は、対象ビルドの
ソースコメントから収集する。生成ヘッダー由来の通知も原文を保持するが、
そのコメントだけで実行コードのライセンス種別を判定しない。
収集対象外の platform・追加 build tag で配布する場合は、そのビルドの依存と
生成コード内の追加通知を確認し、必要なら生成スクリプトの収集対象を更新する。
ツールは非 Go コードの依存を検査できないため、収集時の警告も確認する。

生成スクリプトは、現在の収集・補完作業を再現する補助として扱う。
`go-licenses` と補完処理がすべての通知を自動収集できるとは限らない。
CI の `--check` は通知ファイルと生成結果の一致を確認するもので、
ライセンス通知の網羅性を保証しない。依存の追加・更新時は、配布元の
ライセンス、追加通知、ソース内の通知も確認し、必要な通知を補完対象に追加する。

バイナリを配布する場合は、`LICENSE` と `THIRD-PARTY-NOTICES.txt` を
同じ配布アーカイブに同梱する。`go install` での利用者にも README から通知を案内する。

1. 直前の GitHub Release と今回指定されたバージョンを確認し、その間のタグ・commit・backlog から対象範囲を確定する。最新のタグや backlog の指定バージョンのグループだけを対象範囲としない。
2. 対象範囲の commit の diff を確認し、`README.md`・`README.ja.md`、CLI ヘルプ、その他の文書の更新要否を確認して、現在の実装・仕様に合わせて必要な更新を行う。README の同期は [scope](scope.md)、CLI ヘルプの同期は [constraints](constraints.md) に従う。
3. `CHANGELOG.md` と `CHANGELOG.ja.md` に、未記載の各バージョンを個別の見出しで追記する。GitHub Release を作成しなかった途中のバージョンも含め、今回のバージョンだけにまとめない。既存の記載も対象範囲に漏れがないか確認する。backlog のタスク文をそのまま写さず、対象 commit の diff に基づく最終的なユーザー影響を書く。
4. 英語・日本語の CHANGELOG への反映と同じコミットで、記載した変更に対応する完了済みのバージョングループを backlog から削除する。未完了のタスクは残す。
5. [verification](verification.md) の必須 gate と変更内容に対応する確認を通し、必要な更新をコミットする。指定バージョンのタグとリリース commit の対応を確認し、タグがなければ作成して、必要な commit とタグを push する。既存の途中のバージョンのタグは保持する。
6. 指定バージョンのタグで `gh release` を作成する。本文は、直前の GitHub Release 以降の主な変更の短い要約と、英語・日本語の CHANGELOG へのリンクとする。リンク先は `main` ではなく、今回のリリースタグのファイルに固定する。
