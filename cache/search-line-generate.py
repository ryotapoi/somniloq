import json
from pathlib import Path

root = Path('.tmp/search-line-benchmark')
inputs = root / 'input' / 'project'
inputs.mkdir(parents=True, exist_ok=True)
cases = {
    'dense_long': 'x' * 1999 + 'a',
    'dense_short': 'x' * 255 + 'a',
    'dense_medium': 'x' * 255 + 'a',
    'sparse_long': 'x' * (2_000_000 - 1) + 'a',
    'no_match': 'x' * 2_000_000,
    'multiline': ('x' * 999 + 'a\n') * 2000,
}
cases['dense_long'] *= 1000
cases['dense_short'] *= 100
cases['dense_medium'] *= 1000
for case, content in cases.items():
    event = {
        'type': 'user', 'uuid': f'{case}-message', 'sessionId': case,
        'timestamp': '2026-10-01T01:00:00Z', 'cwd': '/benchmark',
        'message': {'role': 'user', 'content': content},
    }
    (inputs / f'{case}.jsonl').write_text(json.dumps(event, ensure_ascii=False) + '\n', encoding='utf-8')
(root / 'config.toml').write_text(
    'db = "./result.db"\n[[inputs]]\nname = "bench"\n'
    'source = "claude-code"\nroot = "./input"\n', encoding='utf-8'
)
