# Code Review Agent

Build an agent that reviews code and suggests improvements.

---

## What You'll Build

A code review agent that can:

- Read files from a codebase
- Check for common issues
- Suggest improvements
- Comment on specific lines

---

## Prerequisites

- Bridge running locally
- An API key
- A local code repository to test with

---

## Step 1: Create the Agent

Create `code-review-agent.json`:

```json
{
  "agents": [
    {
      "id": "code-reviewer",
      "name": "Code Reviewer",
      "system_prompt": "You are a senior software engineer doing code review.\n\nYour job is to:\n1. Read the code carefully\n2. Check for: security issues, bugs, performance problems, style issues\n3. Provide specific, actionable feedback\n4. Be constructive and respectful\n\nFormat your response as:\n- **Issues Found:** (list critical problems)\n- **Suggestions:** (improvements)\n- **Questions:** (things you're unsure about)\n\nIf you find no issues, say so clearly.",
      "provider": {
        "provider_type": "anthropic",
        "model": "claude-sonnet-4-20250514",
        "api_key": "YOUR_API_KEY"
      },
      "tools": ["read", "glob", "grep"],
      "mcp_servers": [
        {
          "name": "filesystem",
          "transport": {
            "type": "stdio",
            "command": "npx",
            "args": ["@modelcontextprotocol/server-filesystem", "/home/user/projects"]
          }
        }
      ],
      "skills": [
        {
          "id": "security-review",
          "title": "Security Review",
          "description": "Focus on security issues only",
          "content": "Review this code specifically for security issues. Look for: SQL injection, XSS, unsafe deserialization, hardcoded secrets, improper auth checks."
        },
        {
          "id": "performance-review",
          "title": "Performance Review",
          "description": "Focus on performance issues",
          "content": "Review this code for performance issues. Look for: N+1 queries, inefficient algorithms, memory leaks, unnecessary allocations."
        }
      ],
      "config": {
        "max_tokens": 4096,
        "max_turns": 30,
        "temperature": 0.2
      },
      "version": "1"
    }
  ]
}
```

---

## Step 2: Push the Agent

```bash
curl -X POST http://localhost:8080/push/agents \
  -H "Authorization: Bearer <bridge-control-plane-api-key>" \
  -H "Content-Type: application/json" \
  -d @code-review-agent.json
```

---

## Step 3: Review a File

Create a conversation and ask for a review:

```bash
# Create conversation
curl -X POST http://localhost:8080/agents/code-reviewer/conversations \
  -H "Content-Type: application/json" \
  -d '{"user_id": "dev-123"}'

# Request review
curl -X POST http://localhost:8080/conversations/CONV_ID/messages \
  -H "Content-Type: application/json" \
  -d '{
    "role": "user",
    "content": "Please review /home/user/projects/myapp/src/auth.js"
  }'
```

Watch the stream to see the review.

---

## Step 4: Review Multiple Files

Ask for a broader review:

```bash
curl -X POST http://localhost:8080/conversations/CONV_ID/messages \
  -H "Content-Type: application/json" \
  -d '{
    "role": "user",
    "content": "Review all JavaScript files in /home/user/projects/myapp/src for security issues"
  }'
```

---

## Step 5: Use Security Skill

Ask the agent to focus on security:

```bash
curl -X POST http://localhost:8080/conversations/CONV_ID/messages \
  -H "Content-Type: application/json" \
  -d '{
    "role": "user",
    "content": "Use the security-review skill on /home/user/projects/myapp/src/api.js"
  }'
```

---

## What You Learned

- Using filesystem tools with MCP
- Creating specialized skills
- Configuring agents for technical tasks
- Reviewing code with AI

---

## Next Steps

- Connect to GitHub/GitLab to review PRs
- Add more skills (performance, style, tests)
- Create subagents for different review types
- Integrate with CI/CD pipelines

---

## See Also

- [MCP](../core-concepts/mcp.md)
- [Skills](../core-concepts/skills.md)
- [spawn_agent](../tools-reference/agent-tools.md)
