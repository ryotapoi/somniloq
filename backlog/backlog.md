# Backlog

## この backlog の運用ルール

### バージョンと見出し

- バージョン番号は SemVer に従う（機能追加 = minor、修正のみ = patch、破壊的変更 = major）
- 見出しはバージョン単位で切る。リリースとして出す価値のあるまとまりで区切り、goal の大きさには合わせない
- バージョン見出しが大きくなったら、配下にサブ見出しを立てて塊ごとに分ける。**サブ見出し 1 つが 1 goal の実行単位**（数コミットで終わる大きさ）。小さいバージョンならサブ見出しを作らず、バージョン見出しごと 1 goal にしてよい
- **未リリースのバージョン番号は挿入・繰り下げしてよい**。v0.10.0 と v0.11.0 がある状態で v0.10.0 直後にやりたい作業ができたら、それを新 v0.11.0 とし、既存 v0.11.0 を v0.12.0 にずらす。タグを打ったバージョンは動かせない
- 番号が決まらないタスクは番号なしの見出し（例: `## docs`）に置く。次のリリースへ同梱するか独立バージョンにするかは、タグを打つときに決める
- 挙動が変わらない変更（リファクタ・テスト・ドキュメント）だけでバージョンを刻まない

### タグと CHANGELOG

- タグと GitHub Release は 1:1 で作る
- CHANGELOG は**リリース時にまとめて書く**。そのバージョンの項目が全て `[x]` になってから `## vX.Y.Z — YYYY-MM-DD` に書く
- `## Unreleased` 見出しは常設しない（空の見出しを置いておかない）。リリース前に書き溜める必要が出た例外時だけ足し、リリース時にバージョン見出しへ畳む
- CHANGELOG は英語（`CHANGELOG.md`）と日本語（`CHANGELOG.ja.md`）の両方を同じコミットで更新する
- **書くときは各 commit の diff を読む**。backlog のタスク文をそのまま写さない。内部整理のつもりの項目でも、エラー文言等のユーザー影響が出ることがある
- 同一バージョン内で「A を作って後で X に変えた」場合は、A に触れず X だけを書く。前のバージョンの A を変えた場合は変更として書く
- 完了項目は `- [x]` にして残し、英語・日本語の CHANGELOG にそのバージョンを追加する同じコミットで、バージョン見出しごと削除する。内容は CHANGELOG と commit に残る

## v0.10.0 — Cursor Agent ログ対応

調査済みの前提: Cursor Agent `2026.09.10-fd3934a` を観測し、607 files / 9259 records を確認した。観測 path `~/.cursor/projects/<project-slug>/agent-transcripts/<session-id>/<session-id>.jsonl` は全件一致し、session directory と file name も一致する。record は `role=user|assistant` と `message.content` の text / tool_use、終了を示す top-level `type=turn_ended` からなり、独立した timestamp、cwd、repository、version、title、usage、parent 情報はない。controlled follow-up では既存 prefix を保った末尾追記を確認した。project slug は全件を信頼して可逆復元できない。

### Cursor Agent ログ契約を fixture と仕様へ固定する

- [ ] 機密本文を除いた代表 fixture と `docs/specs/jsonl-schema.md` に観測事実を記録し、理由が将来制約になる場合は ADR を追加する。
  - 受理対象 path と session 識別を固定する。user/assistant の text は順序を保って扱い、tool_use、turn_ended、未知の正常 record は会話本文にしない。壊れた JSON と既知 role の不正 content は既存の unparsed 契約で扱う。
  - 同じ入力を再処理しても session/message は重複せず、順序は安定する。ログにない metadata を事実として捏造しない。fixture と記録に機密本文を残さない。
  - `<timestamp>` / `<user_query>` / `<cwd>` の内部 tag を metadata 化または除去するか、private-tmp を含めるか、未知 metadata を内部でどう表すかは backlog で固定しない。実装時に調査事実と既存契約から Minimal Change を決める。これは追加のログ形式調査ではなく設計判断である。

### Cursor Agent を既存 import に統合する

- [ ] ユーザー向け `--source cursor-agent` と default all で Cursor Agent を取り込めるようにする。
  - source 単独と all で保存でき、root 不存在は既存の未使用 source として扱う。malformed を含んでも既存の import error / unparsed 契約に従う。
  - 追記後の再 import は新規会話だけを追加し、既存データを重複させない。縮小と `--full` は既存契約を維持し、Claude Code / Codex import を回帰させない。

### 横断参照と利用者向け文書を同期する

- [ ] sessions / projects / search / show / outline で Cursor を他 source と同じ DB から扱えるようにする。
  - source を区別でき、同じ session ID の ambiguity は既存契約に従う。欠落 metadata を誤表示または誤 filter せず、既存 source を回帰させない。
  - `docs/rules/mission.md`、`docs/rules/scope.md`、README 英日、CLI help など、実際に影響する正本または派生文書だけを同期する。`docs/decisions/` と `llm-wiki/` は必要な場合に更新する。検証は `docs/rules/verification.md` に従う。

### 既知の制約

- local format は公式契約ではなく version 依存である。形式変化が観測された時に再調査する。
- timestamp / repository metadata、usage、親子、tool 履歴はログから完全復元できない。v0.10.0 でどこまで扱うかは goal ごとの Minimal Change で判断する。ただし、捏造しない。
