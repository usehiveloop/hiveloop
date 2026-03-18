# Skill Tool

Load a skill to apply specialized expertise.

---

## Overview

The skill tool lets an agent load a pre-defined skill (reusable prompt). This is useful when:

- An agent needs to switch modes (code → review → explain)
- You want reusable expertise across agents
- Tasks are better handled with focused prompts

---

## Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `name` | string | Yes | Skill ID to load |
| `args` | string | No | Additional context for the skill |

## Example

```json
{
  "name": "skill",
  "arguments": {
    "name": "commit",
    "args": "fix: handle null pointer in user authentication"
  }
}
```

---

## How It Works

1. Agent calls `skill` with a skill name
2. Bridge looks up the skill definition
3. Skill content is made available to the agent
4. Agent uses that expertise for the current task

---

## Defining Skills

Skills are defined in your agent configuration:

```json
{
  "id": "my-agent",
  "skills": [
    {
      "id": "commit",
      "title": "Write Commit Message",
      "description": "Write clear commit messages",
      "content": "You are a commit message writer. Analyze the changes and write a clear, concise commit message following Conventional Commits format."
    },
    {
      "id": "review",
      "title": "Code Review",
      "description": "Review code thoroughly",
      "content": "You are a senior engineer doing code review. Check for: security issues, logic errors, performance problems, and style issues."
    }
  ]
}
```

---

## Parameter Substitution

Skills can include `{{args}}` placeholders:

```json
{
  "id": "commit",
  "content": "Write a commit message for these changes: {{args}}"
}
```

When called with:

```json
{
  "name": "skill",
  "arguments": {
    "name": "commit",
    "args": "Fixed the login bug"
  }
}
```

The content becomes:

```
Write a commit message for these changes: Fixed the login bug
```

### Without Placeholders

If a skill doesn't have `{{args}}`, args are appended:

```json
{
  "id": "review",
  "content": "You are a code reviewer."
}
```

With args `"Review this function"`:

```
You are a code reviewer.

Additional context: Review this function
```

---

## Use Cases

### Code Review

```
User: "Review this PR"
Agent: skill("review") + reads files + provides review
```

### Writing Commit Messages

```
User: "Write a commit message"
Agent: skill("commit", args="changes summary") + generates message
```

### Explaining Code

```
User: "Explain how this works"
Agent: skill("explain") + reads code + explains
```

---

## Skills vs System Prompt

| | System Prompt | Skills |
|---|---------------|--------|
| Applied | Every message | On demand |
| Purpose | Core behavior | Specialized tasks |
| Size | Keep short | Can be detailed |

Use system prompt for: personality, constraints, general instructions

Use skills for: specific tasks that aren't always needed

---

## Example: Multi-Mode Agent

```json
{
  "id": "senior-engineer",
  "system_prompt": "You are a senior engineer. Help with coding tasks.",
  "skills": [
    {
      "id": "code",
      "content": "Write clean, well-tested code. Follow project conventions."
    },
    {
      "id": "review",
      "content": "Review code thoroughly. Check: security, correctness, performance."
    },
    {
      "id": "explain",
      "content": "Explain code simply. Use analogies. Start with big picture."
    }
  ]
}
```

Agent can switch modes by using different skills.

---

## Error: Skill Not Found

If an agent tries to use a skill that doesn't exist:

```json
{
  "success": false,
  "error": "Skill 'unknown-skill' not found"
}
```

Make sure the skill is defined in the agent's `skills` array.

---

## See Also

- [Skills](../core-concepts/skills.md) — Concept overview
- [Agents](../core-concepts/agents.md) — Agent configuration
