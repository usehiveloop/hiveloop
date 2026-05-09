"""Read current config from stdin, apply defaults, print updated config to stdout."""
import json
import sys

d = json.load(sys.stdin)

sp = d['agent']['system_prompt']
if 'load_tools' not in sp:
    sp += ('\n\nMCP tools are lazy-loaded. Call load_tools(tool_names=[...]) '
           'with ALL tools you need in ONE call. Loaded tools appear on the next response.')

if 'When working on tasks' not in sp:
    sp += ('\n\nWhen working on tasks that require multiple tool calls:\n'
           '- Call post_status_update at the START to tell the user what you\'re doing\n'
           '- Call post_status_update if the task takes longer than expected\n'
           '- Call post_status_update at the END with a summary\n'
           '- Never repeat post_status_update content in your final reply')

d['agent']['system_prompt'] = sp

tools = d.get('tools', [])
existing = {t['type'] for t in tools}
for spec in [
    'builtin.post_status_update', 'builtin.post_to_channel',
    'builtin.cron', 'builtin.delegate', 'builtin.check_delegated_status',
    'builtin.check_bash_status', 'builtin.wake', 'builtin.load_tools',
]:
    if spec not in existing:
        tools.append({'type': spec})
d['tools'] = tools

if 'context' not in d:
    d['context'] = {}
if d['context'].get('compaction') is None:
    d['context']['compaction'] = {
        'enabled': True,
        'token_threshold': 90000,
        'overlap_event_count': 10,
        'chars_per_token': 4,
        'summarizer_model': d['model'],
    }

print(json.dumps(d))
