---
generated: 2026-10-12
baseline_commit: 46588740126700d9e5d847a11b70951c25763aa0
candidate: この記録と同じ差分の show.go と db_messages_summary.go
---

# show の少量表示

採用: role・発言番号の絞込みを本文取得 SQL に反映し、日時条件のない page / tail は各 REF の対象件数を数えた後、必要な本文と blocks だけ取得する。日時条件は raw timestamp の RFC3339 解釈を Go で維持する。REF 入力順、重複除去、filter→page の順序、total、blocks と出力は維持する。schema と保存形式は変えない。

## 条件と結果

macOS Darwin 27.0.0 arm64、Go 1.27.1、通常 build、ローカル SQLite、温かい OS cache。合成 Codex 1 会話 2,000 件、各本文約 4 KiB、全本文＋blocks JSON は 16,441,786 bytes。JSON 出力をファイルへ書き、起動から終了までの wall / CPU と `wait4` peak RSS を交互に各 6 回測った。各回 exit 0、出力 SHA-256 は両版で一致。中央値（最小–最大）、wall は ms、RSS は MiB。

| 操作 | baseline wall | candidate wall | baseline RSS | candidate RSS | Go が取得・decode した本文件数 |
|---|---:|---:|---:|---:|---:|
| `--messages 1996:2000` | 62.0 (60.8–63.4) | 20.2 (19.4–21.1) | 50.7 (49.4–51.3) | 17.8 (17.7–17.9) | 2,000→5 |
| `--limit 5` | 63.5 (60.4–79.0) | 24.5 (24.2–25.3) | 50.8 (49.4–51.3) | 18.4 (18.3–18.7) | 2,000→5 |
| `--tail 5` | 63.6 (61.7–70.8) | 62.1 (59.4–84.0) | 50.4 (50.0–51.1) | 22.7 (22.5–22.8) | 2,000→5 |
| 全件 | 105.1 (95.3–119.5) | 104.9 (93.7–137.1) | 159.2 (139.5–163.4) | 158.6 (141.3–168.2) | 2,000→2,000 |

少量取得時に Go が受け取る本文＋blocks は 16.44 MB から約 41 KB に減る。これは SQLite の物理 I/O 量ではない。`--tail` は件数 query と `ORDER BY ... LIMIT ... OFFSET ...` が残り、`EXPLAIN QUERY PLAN` は session index の探索と一時 B-tree sort を示したため、wall は明確に短縮しなかった。CPU 中央値は 69.0→59.8 ms、wall はばらつきが重なる。全件の速度・メモリにも測定可能な差はなかった。tail の追加最適化は索引または逆順取得の新たな経路を要し、この範囲では採用しない。

小さい案（role・番号を既存取得へ反映）は `--messages` の本文 read と decode を減らす。総件数・複数 REF・tail・page を扱う案は count と REF ごとの page 配分を加え、`--limit` の wall と少量表示の RSS に効果があった。後者の追加負担は局所的な count / page 経路と SQL 条件の共有で収まり、明確な全件速度悪化は観測されなかったため採用した。日時指定付き page は全件を Go で時刻判定するため少量取得の改善対象外。

正しさ: 既存と追加の CLI / core テストで filter、複数 REF、tail、offset、legacy blocks を確認。基準版との JSON byte 一致は上記 4 操作と、重複 REF、日時＋page、role＋番号＋page、tail=0、limit=0 でも確認した。CLI 実行で成功・入力エラーの stdout / stderr / 終了値を確認した。共通 gate の format、`go test -count=1 ./...`、`go vet ./...`、`go build -o bin/somniloq ./cmd/somniloq`、`go install github.com/google/go-licenses/v2@v2.0.1`、通知 check はすべて終了 0。`go install` は sandbox の DNS 拒否後、権限拡張で成功した。

限界: 合成した単一 Codex 会話・温かい cache の測定であり、多数 REF、legacy の長い履歴、cold disk、巨大 blocks、他機種は未計測。計測予算は 1 process 10 秒、RSS 512 MiB、一時出力 1 GiB とし、軽い 2,000 件から始めて拡大せず終了した。候補の tail wall に 84 ms の外れ値が一回あり、数 ms の差は確定できない。

## 再実行

`cache/show-performance-measure.py` は生成時点の計測 helper。baseline と candidate を `.tmp/show-performance/` に build し、同じ seed DB に対して走らせる。基準版の source は次で隔離する。

```sh
mkdir -p .tmp/show-performance/baseline-source .tmp/show-performance/logs
git archive 46588740126700d9e5d847a11b70951c25763aa0 | tar -x -C .tmp/show-performance/baseline-source
(cd .tmp/show-performance/baseline-source && go build -o ../baseline ./cmd/somniloq)
go build -o .tmp/show-performance/candidate ./cmd/somniloq
python3 - <<'PY'
import json, pathlib
p=pathlib.Path('.tmp/show-performance')
(p/'config.toml').write_text("db = 'show.db'\n[[inputs]]\nsource = 'codex'\nroot = 'logs'\n")
with (p/'logs'/'long.jsonl').open('w') as f:
    f.write(json.dumps({'type':'session_meta','timestamp':'2026-10-01T00:00:00Z','payload':{'id':'show-long','timestamp':'2026-10-01T00:00:00Z','cwd':'/fixtures/project'}})+'\n')
    for i in range(1,2001):
        role='user' if i%2 else 'assistant'
        kind='input_text' if role=='user' else 'output_text'
        f.write(json.dumps({'type':'response_item','payload':{'type':'message','id':f'm{i}','role':role,'content':[{'type':kind,'text':f'message {i} '+ 'x'*4096}]},'ordinal':i,'timestamp':'2026-10-01T00:01:00Z'})+'\n')
PY
.tmp/show-performance/baseline import --config .tmp/show-performance/config.toml
python3 cache/show-performance-measure.py > .tmp/show-performance/paired.jsonl
```

helper は各操作を baseline / candidate 交互に 6 対で実行し、終了値、出力 hash、件数、wall、CPU、peak RSS を JSONL に残す。seed DB は再作成してから使う。
