# 開発時の取り決め

コード・コメント・コミットメッセージは英語、`AGENTS.md`・`.agents/`・`docs/`・`decisions/`・`cache/`・`backlog/`・README 等の文書は日本語で書く。既存の英語・日本語 README / CHANGELOG の対は維持する。

`--no-verify` で Git hook を飛ばさない。明示指示なしに force push しない。作業中の一時物は gitignore された `.tmp/` に置き、消えても製品・検証が壊れないようにする。変更の検証は [検証手順](verification.md) に従う。
