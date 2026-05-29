---
name: rails-qa-evidence
description: QA evidence workflow for Rails apps using bin/rails commands, local server startup, agent-browser screenshots, recordings, logs, and a qa-report.md.
category: testing
tags: [qa, rails, browser, screenshots, video]
---

# Rails QA Evidence

Use this skill for QA validation of `/workspace/app`.

## Run Directory

Create one run directory per validation:

```bash
RUN_ID="$(date +%Y%m%d-%H%M%S)"
QA_DIR="/workspace/qa-runs/$RUN_ID"
mkdir -p "$QA_DIR/screenshots" "$QA_DIR/recordings" "$QA_DIR/logs"
```

## Command Evidence

For web applications:

```bash
cd /workspace/app
bin/rails test
```

For route-heavy changes:

```bash
bin/rails routes > "$QA_DIR/logs/routes.txt"
```

Start the server:

```bash
bin/rails server -b 0.0.0.0 -p 3000 > "$QA_DIR/logs/rails-server.log" 2>&1 &
SERVER_PID=$!
until curl -sf http://127.0.0.1:3000 >/dev/null 2>&1; do sleep 1; done
```

## Browser Evidence

Use `agent-browser` directly:

```bash
agent-browser open http://127.0.0.1:3000
agent-browser snapshot -i
agent-browser screenshot "$QA_DIR/screenshots/01-home.png"
agent-browser record start "$QA_DIR/recordings/happy-path.webm"
# interact through the app
agent-browser record stop
```

Capture happy path, empty state, validation failure, success state, persistence after refresh, destructive actions, and responsive screenshots when relevant.

## Report

Write `$QA_DIR/qa-report.md` with:

- Summary
- Commands and pass/fail
- Screenshot paths
- Recording paths
- Bugs with reproduction steps
- Untested risk

## Source Notes

Adapted from official/public web-app testing skill patterns and the local `agent-browser` skill, rewritten for Rails and the Hivy eval artifact layout.
