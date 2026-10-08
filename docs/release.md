# リリース

## 版とタグ

版番号には SemVer を使う。版を上げてタグを打つ変更では、対象コミットの差分と完了済み backlog タスクを照合する。完了タスクを削除する前に、今後も必要な理由・制約がコード・テスト・文書・現行 ADR に残っているか確認する。未完了タスクは残す。外部に配る版は同じ変更で `CHANGELOG.md` と `CHANGELOG.ja.md` にユーザーへの影響を記録する。タスク文をそのまま転記せず、最終差分を基準に書く。

README 両言語と CLI help の同期を確認し、[検証手順](verification.md) の共通 gate と変更条件に応じた確認を通す。タグと版上げコミットの対応を確認する。タグだけの版に GitHub Release は不要である。

## GitHub Release

GitHub Release は明示的に指定された版だけで作成する。直前の GitHub Release から今回の版までのタグとコミットを確認し、途中の版も個別に CHANGELOG に記録する。指定タグで `gh release` を作成し、本文には主な変更の短い要約と、そのタグに固定した英語・日本語 CHANGELOG へのリンクを置く。

依存関係または Go version を変更した版では、Python 3.9 以降で通知を再生成し、差分のライセンス原文・著作権表示・追加通知を確認する。

```bash
go install github.com/google/go-licenses/v2@v2.0.1
python3 scripts/update-third-party-notices.py
```

`THIRD-PARTY-NOTICES.txt` の生成スクリプトと CI の `--check` は網羅性を保証しない。依存追加・更新時は配布元のライセンスとソース内通知を確認し、収集時の非 Go コードに関する警告も確認する。対象外 platform や追加 build tag を配布するときは、その依存と生成コードの通知を調べ、必要に応じてスクリプトの収集対象を更新する。バイナリの配布アーカイブには `LICENSE` と `THIRD-PARTY-NOTICES.txt` を同梱し、`go install` 利用者には README から通知を案内する。
