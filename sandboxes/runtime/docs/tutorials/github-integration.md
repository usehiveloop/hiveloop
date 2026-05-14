# GitHub Integration

Build an agent that creates pull requests.

---

## What You'll Build

An agent that can:

- Read your codebase
- Make changes
- Create a branch
- Commit changes
- Open a pull request

---

## Prerequisites

- Bridge running locally
- GitHub API token
- Local git repository
- Control plane configured with GitHub integration endpoints

---

## Step 1: Create the Agent

`github-agent.json`:

```json
{
  "agents": [
    {
      "id": "github-assistant",
      "name": "GitHub Assistant",
      "system_prompt": "You are a helpful assistant that helps users create pull requests.\n\nWhen asked to make changes:\n1. Read the relevant files\n2. Make the necessary edits\n3. Create a git branch\n4. Commit the changes\n5. Push the branch\n6. Create a pull request\n\nAlways ask for confirmation before creating PRs.",
      "provider": {
        "provider_type": "anthropic",
        "model": "claude-sonnet-4-20250514",
        "api_key": "YOUR_API_KEY"
      },
      "tools": ["Read", "write", "edit", "bash"],
      "integrations": [
        {
          "name": "github",
          "description": "GitHub API integration",
          "actions": [
            {
              "name": "create_pull_request",
              "description": "Create a pull request",
              "parameters_schema": {
                "type": "object",
                "properties": {
                  "owner": { "type": "string" },
                  "repo": { "type": "string" },
                  "title": { "type": "string" },
                  "body": { "type": "string" },
                  "head": { "type": "string" },
                  "base": { "type": "string" }
                },
                "required": ["owner", "repo", "title", "head", "base"]
              },
              "permission": "require_approval"
            }
          ]
        }
      ],
      "permissions": {
        "bash": "require_approval"
      },
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

Replace `YOUR_API_KEY`. The GitHub integration is provided by your control plane at `/integrations/github/actions/{action_name}`.

---

## Step 2: Push the Agent

```bash
curl -X POST http://localhost:8080/push/agents \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d @github-agent.json
```

---

## Step 3: Make a Change

Create a conversation and ask for a change:

```bash
curl -X POST http://localhost:8080/agents/github-assistant/conversations \
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
    "content": "Add a README to /home/user/projects/myapp. The project is a task manager built with React."
  }'
```

The agent will:
1. Check if README exists
2. Create one with project info
3. Stage, commit, and push changes (via bash — requires approval)
4. Create a PR (requires approval)

---

## Step 4: Approve Actions

List pending approvals:

```bash
curl http://localhost:8080/agents/github-assistant/conversations/CONV_ID/approvals
```

Approve the bash commands:

```bash
curl -X POST http://localhost:8080/agents/github-assistant/conversations/CONV_ID/approvals \
  -H "Content-Type: application/json" \
  -d '{
    "request_ids": ["req-bash-123"],
    "decision": "approve"
  }'
```

Then approve the PR creation:

```bash
curl -X POST http://localhost:8080/agents/github-assistant/conversations/CONV_ID/approvals \
  -H "Content-Type: application/json" \
  -d '{
    "request_ids": ["req-github-456"],
    "decision": "approve"
  }'
```

---

## What You Learned

- Integrating with GitHub via control plane
- Using bash tool with approvals
- Creating integration tools
- Building a complete workflow

---

## Next Steps

- Add more GitHub actions (list PRs, merge, comment)
- Integrate with GitLab or Bitbucket
- Add CI status checks
- Auto-assign reviewers

---

## See Also

- [Integration Tools](../tools-reference/custom-tools.md)
- [bash tool](../tools-reference/bash-tool.md)
- [Agents API](../api-reference/agents-api.md) — Approval endpoints
