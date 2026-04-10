# Slack webhook fixtures

All fixtures are either pulled verbatim from authoritative Slack sources or explicitly derived from real payloads for test scenarios that the canonical sources don't ship directly.

| File | Source | Notes |
|---|---|---|
| `app_mention.top_level.json` | `slackapi/bolt-python` `tests/scenario_tests/test_events.py` (`valid_event_body`) | Slack's own SDK test fixture. @mention at channel top level (no `event.thread_ts`). Used as the canonical parent of thread continuation tests. |
| `app_mention.in_thread.json` | **Derived** from `app_mention.top_level.json` | Adds `event.thread_ts = "1595926230.009600"` matching the top-level fixture's `ts` to simulate a user mentioning the bot inside an existing thread. This is the key fixture for validating coalescing refs — a top-level mention and an in-thread mention on the same thread must produce the same resource key. |
| `message.channels.json` | `docs.slack.dev/reference/events/message.channels` | Official docs inline example. Top-level channel message, no `thread_ts`. |
| `message.channels.thread_reply.json` | **Derived** from `message.channels.json` with a thread_ts matching `app_mention.top_level.json`'s `ts` | A follow-up reply in the same thread as the canonical app_mention fixture. Tests "new message in a thread the agent is tracking" continuation scenarios. |
| `message.im.json` | `docs.slack.dev/reference/events/message.im` | Official docs inline example. Direct message to the bot (`channel_type = "im"`). |
| `reaction_added.json` | `slackapi/bolt-python` `tests/scenario_tests/test_events.py` (`valid_reaction_added_body`) | Slack's own SDK test fixture. Reaction added to a message, with `item.channel` and `item.ts` identifying the reacted-to message. |
| `member_joined_channel.json` | `slackapi/bolt-python` `tests/scenario_tests/test_events.py` | Slack's own SDK test fixture. User joined a channel (used for welcome-flow agents). |
| `message.file_share.json` | `slackapi/bolt-python` `tests/scenario_tests/test_events.py` (`message_file_share_body`) | Slack's own SDK test fixture, trimmed. Message event with `subtype = "file_share"` and attached files. |

## Envelope shape

All Slack webhooks share the same outer envelope:

```
{
  "token": "...",
  "team_id": "T...",
  "enterprise_id": "E...",           // optional, enterprise installs only
  "api_app_id": "A...",
  "event": { /* the per-event data, shape varies */ },
  "type": "event_callback",
  "event_id": "Ev...",
  "event_time": 1234567890,
  "authorizations": [ /* ... */ ]
}
```

The inner `event` object is where event-specific fields live. Ref paths in the trigger catalog always start with `event.` (e.g., `event.channel`, `event.thread_ts`).

## The `thread_ts` asymmetry

Slack's `event.thread_ts` is **only present** when the message is a reply inside an existing thread. For top-level channel messages and mentions, the field is absent — only `event.ts` exists.

To produce a consistent thread identifier across both cases, the trigger catalog uses a **coalescing ref**:

```json
"thread_id": "event.thread_ts || event.ts"
```

The dispatcher tries `event.thread_ts` first and falls back to `event.ts` if it's missing. This means a top-level @mention (which will become a thread root once replies arrive) and any subsequent reply in that thread both resolve to the same `thread_id`, which means the same `resource_key`, which means the executor finds the same conversation.

The pair of fixtures `app_mention.top_level.json` + `app_mention.in_thread.json` is specifically designed to exercise this: their `event.thread_ts` (on the second) matches the first's `event.ts`, and the `thread_id` coalescing ref is what makes them produce the same resource key.
