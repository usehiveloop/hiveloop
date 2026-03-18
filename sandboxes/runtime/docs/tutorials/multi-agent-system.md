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

## Step 1: Create the Parent Agent with Subagents

Create `multi-agent-system.json` with all agents defined together:

```json
{
  "agents": [
    {
      "id": "senior-engineer",
      "name": "Senior Engineer",
      "system_prompt": "You are a senior engineer coordinating a team. When given code to work on:\n1. Delegate review to the code_reviewer subagent\n2. Delegate tests to the test_writer subagent\n3. Delegate docs to the documenter subagent\n4. Collect results and present a summary\n\nUse parallel_agent tool for efficiency.",
      "provider": {
        "provider_type": "anthropic",
        "model": "claude-sonnet-4-20250514",
        "api_key": "YOUR_API_KEY"
      },
      "tools": ["Read", "parallel_agent", "join"],
      "subagents": [
        {
          "id": "sub-reviewer",
          "name": "code_reviewer",
          "description": "Code reviewer that checks for bugs, security, and style issues",
          "system_prompt": "You are a code reviewer. Review code for bugs, security, and style issues. Provide specific, actionable feedback.",
          "provider": {
            "provider_type": "anthropic",
            "model": "claude-haiku-4-5-20251001",
            "api_key": "YOUR_API_KEY"
          },
          "tools": ["Read"],
          "config": {
            "max_tokens": 2048,
            "temperature": 0.2
          }
        },
        {
          "id": "sub-tester",
          "name": "test_writer",
          "description": "Test generation specialist",
          "system_prompt": "You write unit tests. Given code, write comprehensive tests covering normal cases, edge cases, and error cases.",
          "provider": {
            "provider_type": "anthropic",
            "model": "claude-haiku-4-5-20251001",
            "api_key": "YOUR_API_KEY"
          },
          "tools": ["Read", "write"],
          "config": {
            "max_tokens": 2048,
            "temperature": 0.3
          }
        },
        {
          "id": "sub-docs",
          "name": "documenter",
          "description": "Documentation writer",
          "system_prompt": "You write documentation. Given code, write clear docstrings and usage examples.",
          "provider": {
            "provider_type": "anthropic",
            "model": "claude-haiku-4-5-20251001",
            "api_key": "YOUR_API_KEY"
          },
          "tools": ["Read"],
          "config": {
            "max_tokens": 2048,
            "temperature": 0.4
          }
        }
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

Replace `YOUR_API_KEY` with your actual API key.

---

## Step 2: Push the Agent

```bash
curl -X POST http://localhost:8080/push/agents \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d @multi-agent-system.json
```

---

## Step 3: Test the System

Create a conversation and ask for a full review:

```bash
curl -X POST http://localhost:8080/agents/senior-engineer/conversations \
  -H "Content-Type: application/json" \
  -d '{"user_id": "dev-123"}'
```

Connect to the stream:

```bash
curl -N http://localhost:8080/conversations/CONV_ID/stream \
  -H "Accept: text/event-stream"
```

Send the request:

```bash
curl -X POST http://localhost:8080/conversations/CONV_ID/messages \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Please do a full review of /home/user/projects/myapp/src/utils.js - I need code review, tests, and documentation"
  }'
```

Watch the parent agent:
1. Read the file
2. Spawn 3 subagents in parallel using `parallel_agent`
3. Collect results
4. Present summary

---

## How It Works

The parent agent uses the `parallel_agent` tool to delegate tasks:

```json
{
  "name": "parallel_agent",
  "arguments": {
    "max_concurrent": 3,
    "tasks": [
      {
        "subagent": "code_reviewer",
        "description": "Review utils.js",
        "prompt": "Review /home/user/projects/myapp/src/utils.js for bugs and security issues"
      },
      {
        "subagent": "test_writer",
        "description": "Write tests for utils.js",
        "prompt": "Write comprehensive unit tests for /home/user/projects/myapp/src/utils.js"
      },
      {
        "subagent": "documenter",
        "description": "Document utils.js",
        "prompt": "Write clear documentation for /home/user/projects/myapp/src/utils.js"
      }
    ]
  }
}
```

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
