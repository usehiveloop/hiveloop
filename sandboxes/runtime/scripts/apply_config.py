"""Read config from stdin, apply sane defaults, print updated config to stdout."""
import json
import re
import sys

d = json.load(sys.stdin)

sp = d['agent']['system_prompt']

sp = re.sub(
    r'\n*\nMCP tools are lazy-loaded.*?final reply',
    '',
    sp,
    flags=re.DOTALL
).strip()

sp += '\n\nMCP tools are lazy-loaded. Call load_tools(tool_names=[...]) with ALL tools you need in ONE call. Loaded tools appear on the next response.'
d['agent']['system_prompt'] = sp

tools = d.get('tools', [])
existing = {t['type'] for t in tools}
for spec in [
    'builtin.cron', 'builtin.delegate', 'builtin.check_delegated_status',
    'builtin.check_bash_status', 'builtin.wake', 'builtin.load_tools',
]:
    if spec not in existing:
        tools.append({'type': spec})
d['tools'] = tools

if 'context' not in d:
    d['context'] = {}
comp = d['context'].get('compaction')
if comp is None:
    d['context']['compaction'] = {
        'enabled': True,
        'token_threshold': 90000,
        'overlap_event_count': 10,
        'chars_per_token': 4,
        'summarizer_model': d['model'],
    }

print(json.dumps(d))
