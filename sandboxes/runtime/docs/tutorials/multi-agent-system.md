# Multi-Agent System

Build a parent agent that delegates to specialized subagents.

---

## What You'll Build

A system with:

- **Parent agent** — Coordinates work
- **Code reviewer** — Reviews code
- **Test writer** — Writes tests
- **Documenter** — Writes documentation

The parent delegates tasks to subagents in parallel.

---

## Prerequisites

- Bridge running locally
- API key
- Sample code to work with

---

## Step 1: Create Subagents

### Code Reviewer

`subagent-reviewer.json`:

```json
{
  "agents": [
    {
      "id": "sub-reviewer",
      "name": "Code Reviewer",
      "system_prompt": "You are a code reviewer. Review code for bugs, security, and style issues. Provide specific, actionable feedback.",
      "provider": {
        "provider_type": "anthropic",
        "model": "claude-haiku-4-5-20251001",
        "api_key": "YOUR_API_KEY"
      },
      "tools": ["read"],
      "config": {
        "max_tokens": 2048,
        "temperature": 0.2
      },
      "version": "1"
    }
  ]
}
```

### Test Writer

`subagent-tester.json`:

```json
{
  "agents": [
    {
      "id": "sub-tester",
      "name": "Test Writer",
      "system_prompt": "You write unit tests. Given code, write comprehensive tests covering normal cases, edge cases, and error cases.",
      "provider": {
        "provider_type": "anthropic",
        "model": "claude-haiku-4-5-20251001",
        "api_key": "YOUR_API_KEY"
      },
      "tools": ["read", "write"],
      "config": {
        "max_tokens": 2048,
        "temperature": 0.3
      },
      "version": "1"
    }
  ]
}
```

### Documenter

`subagent-docs.json`:

```json
{
  "agents": [
    {
      "id": "sub-docs",
      "name": "Documenter",
      "system_prompt": "You write documentation. Given code, write clear docstrings and usage examples.",
      "provider": {
        "provider_type": "anthropic",
        "model": "claude-haiku-4-5-20251001",
        "api_key": "YOUR_API_KEY"
      },
      "tools": ["read"],
      "config": {
        "max_tokens": 2048,
        "temperature": 0.4
      },
      "version": "1"
    }
  ]
}
```

---

## Step 2: Push Subagents

```bash
curl -X POST http://localhost:8080/push/agents \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d @subagent-reviewer.json

curl -X POST http://localhost:8080/push/agents \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d @subagent-tester.json

curl -X POST http://localhost:8080/push/agents \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d @subagent-docs.json
```

---

## Step 3: Create Parent Agent

`parent-agent.json`:

```json
{
  "agents": [
    {
      "id": "senior-engineer",
      "name": "Senior Engineer",
      "system_prompt": "You are a senior engineer coordinating a team. When given code to work on:\n1. Delegate review to the code_reviewer\n2. Delegate tests to the test_writer\n3. Delegate docs to the documenter\n4. Collect results and present a summary\n\nUse parallel_agent for efficiency.",
      "provider": {
        "provider_type": "anthropic",
        "model": "claude-sonnet-4-20250514",
        "api_key": "YOUR_API_KEY"
      },
      "tools": ["read", "parallel_agent", "join"],
      "subagents": [
        { "name": "code_reviewer", "agent_id": "sub-reviewer" },
        { "name": "test_writer", "agent_id": "sub-tester" },
        { "name": "documenter", "agent_id": "sub-docs" }
      ],
      "config": {
        "max_tokens": 4096,
        "max_turns": 50,
        "temperature": 0.3
      },
      "version": "1"
    }
  ]
}
```

Push it:

```bash
curl -X POST http://localhost:8080/push/agents \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d @parent-agent.json
```

---

## Step 4: Test the System

Create a conversation and ask for a full review:

```bash
curl -X POST http://localhost:8080/agents/senior-engineer/conversations \
  -H "Content-Type: application/json" \
  -d '{"user_id": "dev-123"}'

curl -X POST http://localhost:8080/conversations/CONV_ID/messages \
  -H "Content-Type: application/json" \
  -d '{
    "role": "user",
    "content": "Please do a full review of /home/user/projects/myapp/src/utils.js - I need code review, tests, and documentation"
  }'
```

Watch the parent agent:
1. Read the file
2. Spawn 3 subagents in parallel
3. Collect results
4. Present summary

---

## What You Learned

- Creating specialized subagents
- Configuring parent agent with subagents
- Using parallel_agent for efficiency
- Coordinating work across agents

---

## Next Steps

- Add more specialized subagents (security, performance)
- Chain subagents (reviewer → fixer)
- Use cheaper models for subagents
- Add approval gates between steps

---

## See Also

- [Agent Tools](../tools-reference/agent-tools.md)
- [Agents](../core-concepts/agents.md)
