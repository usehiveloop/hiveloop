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
| `name` | string | Yes | Skill ID or title to load (case-insensitive) |
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
2. Bridge looks up the skill definition (matching by ID or title, case-insensitive)
3. Skill content is substituted with args (if provided) and wrapped in XML tags
4. Agent uses that expertise for the current task

Output format:
```xml
<skill_content name="skill-id" title="Skill Title">
Skill content with {{args}} substituted...
</skill_content>
```

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

The `description` field is shown in system reminders to help agents know when to use each skill.

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

If a skill doesn't have `{{args}}`, args are appended with an "Arguments:" prefix:

```json
{
  "id": "review",
  "content": "You are a code reviewer."
}
```

With args `"Review this function"`:

```
You are a code reviewer.

Arguments: Review this function
```

Note: The documentation previously mentioned "Additional context:" but the actual implementation uses "Arguments:".

### Without Args

If no args are provided, the skill content is returned unchanged (including any `{{args}}` placeholders).

---

## Skill Matching

The skill tool matches by **ID or title** (case-insensitive):

| Query | Matches skill with... |
|-------|----------------------|
| `"commit"` | id: `commit` or title: `Commit` |
| `"Code Review"` | id: `code-review` or title: `Code Review` |
| `"PR-SUMMARY"` | id: `pr-summary` or title: `PR Summary` |

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

## Error Handling

### Skill Not Found

If an agent tries to use a skill that doesn't exist, the tool returns an error:

```
Skill 'unknown-skill' not found. Available skills: [Code Review, Commit, Explain]
```

The error includes the list of available skill titles to help identify the correct skill name.

### No Skills Defined

**The skill tool is only registered if the agent has at least one skill defined.** Agents without skills cannot use this tool.

---

## System Reminder Integration

When an agent has skills, Bridge injects a system reminder before every user message:

```xml
<system-reminder>

# System Reminders

## Available skills

The following skills are available for use with the Skill tool:

- **Code Review** - Reviews code for quality, bugs, and best practices
- **Commit** - Writes conventional commit messages

</system-reminder>
```

The reminder shows skill titles and descriptions to help the AI know when to use each skill. The actual skill content is hidden until the skill is invoked.

---

## Blocking Requirement

According to the skill tool instructions, when a skill matches the user's request, invoking the skill tool is a **blocking requirement**. The agent must call the skill tool BEFORE generating any text response about the task.

Additionally:
- Never mention a skill without actually calling the tool
- Do not invoke a skill that is already running (check for `<skill_content>` tags in the current turn)
- Do not use this tool for built-in CLI commands (like /help, /clear, etc.)

---

## Implementation Details

### No Caching

Skills are stored in memory when the agent is initialized. They are not cached to disk or shared between agents.

### No Size Limits

There is no enforced maximum size for skill content. However, very large skills consume context window tokens.

### No Maximum Skill Count

There is no enforced maximum number of skills per agent. However, since all skill titles and descriptions appear in every system reminder, a large number of skills will consume significant context window tokens.

### Case-Insensitive Matching

Skill lookups are case-insensitive for both ID and title matching.

---

## See Also

- [Skills](../core-concepts/skills.md) — Concept overview
- [Agents](../core-concepts/agents.md) — Agent configuration
