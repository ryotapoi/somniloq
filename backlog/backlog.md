# Backlog

## 運用ルール

- Backlog には、今後着手でき、完了を判定できる作業を `- [ ]` 形式のタスクとして置く。
- 対応要否を決める調査もタスクにできる。観察事実だけを残さず、判断したい問いと、判断をもって完了とすることを明示する。
- 粗いメモで起票してよいが、着手前に目的と受け入れ条件を読める粒度へ詰める。
- バージョン番号を付ける場合は SemVer に従う。
- タスクは `###` 見出しでグループに分ける。バージョンが確定したグループだけ、`### v0.11.0 名前` のように見出しへ番号を付ける。
- グループが大きい場合は、`####` 見出しでさらに分割してよい。

## タスク

### 保守

- [x] UTC の `imported_at` fixture とタイムゾーンなしの `--imported-since` を組み合わせる `TestSessionsCmd_ImportedSinceFiltersUnknownStartedAt` および `TestSessionsCmd_ImportedSinceJSONPreservesSourceAndSessionID` が `time.Local` に依存して `America/Los_Angeles` で失敗するため、production の挙動を変えず、テスト内で `time.Local` を UTC に設定・復元して必須テスト gate の誤失敗をなくす。完了条件: 両テストが UTC、Asia/Tokyo、America/Los_Angeles で通り、共通の `go test -count=1 ./...` が成功する。
