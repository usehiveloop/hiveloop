# Hiveloop

## Employee Sandbox SQLite Cleanup

If an employee sandbox has a large backlog of per-token stream rows, stop the
employee runtime, run this inside the sandbox/container, then start the runtime
again:

```bash
sqlite3 /app/data/employee-bridge.db <<'SQL'
PRAGMA busy_timeout = 30000;

BEGIN IMMEDIATE;

DELETE FROM outbound_outbox
WHERE event_type IN ('agent.stream.token', 'agent.stream.thinking');

DELETE FROM events_log
WHERE event_type IN ('agent.stream.token', 'agent.stream.thinking');

COMMIT;

PRAGMA wal_checkpoint(TRUNCATE);
VACUUM;
SQL
```

The closing `SQL` marker must start at the beginning of the line with no leading
spaces.
