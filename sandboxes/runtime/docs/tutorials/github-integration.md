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
      "tools": ["read", "write", "edit", "bash"],
      "integrations": [
        {
          "name": "github",
          "description": "GitHub API integration",
          "base_url": "https://api.github.com",
          "headers": {
            "Authorization": "token YOUR_GITHUB_TOKEN",
            "Accept": "application/vnd.github.v3+json"
          },
          "actions": [
            {
              "name": "create_pull_request",
              "description": "Create a pull request",
              "method": "POST",
              "path": "/repos/{owner}/{repo}/pulls",
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

Replace `YOUR_API_KEY` and `YOUR_GITHUB_TOKEN`.

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

curl -X POST http://localhost:8080/conversations/CONV_ID/messages \
  -H "Content-Type: application/json" \
  -d '{
    "role": "user",
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

Approve the bash commands:

```bash
curl -X POST http://localhost:8080/agents/github-assistant/conversations/CONV_ID/approvals \
  -H "Content-Type: application/json" \
  -d '{"action": "approve_all"}'
```

Then approve the PR creation.

---

## What You Learned

- Integrating with GitHub API
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
