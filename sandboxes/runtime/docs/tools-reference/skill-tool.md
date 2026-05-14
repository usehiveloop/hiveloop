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
| `file` | string | No | Request a specific supporting file by its relative path. Returns only that file's content instead of the full skill. |

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

### Requesting a specific file

```json
{
  "name": "skill",
  "arguments": {
    "name": "use-railway",
    "file": "references/deploy.md"
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

Skills can include `{{args}}` placeholders that are replaced when the skill is invoked. The substitution follows three rules depending on the combination of placeholder presence and args provided.

### Case 1: Content has `{{args}}` AND args are provided

The `{{args}}` placeholder is replaced with the args value.

```json
{
  "id": "commit",
  "content": "Write a commit message for these changes: {{args}}"
}
```

Called with:

```json
{
  "name": "skill",
  "arguments": {
    "name": "commit",
    "args": "Fixed the login bug"
  }
}
```

Result:

```
Write a commit message for these changes: Fixed the login bug
```

### Case 2: Content does NOT have `{{args}}` BUT args are provided

When the content has no placeholder, args are appended as a separate section with an `Arguments:` prefix:

```json
{
  "id": "review",
  "content": "You are a code reviewer."
}
```

Called with args `"Review this function"`:

```
You are a code reviewer.

Arguments: Review this function
```

### Case 3: No args provided

The skill content is returned as-is. If the content contains a `{{args}}` placeholder, it remains in the output unchanged.

```json
{
  "id": "commit",
  "content": "Write a commit message for these changes: {{args}}"
}
```

Called without args:

```
Write a commit message for these changes: {{args}}
```

### Summary

| Content has `{{args}}`? | Args provided? | Behavior |
|--------------------------|----------------|----------|
| Yes | Yes | Placeholder replaced with args value |
| No | Yes | Args appended as `\n\nArguments: {args}` |
| Yes | No | Content returned as-is (placeholder remains) |
| No | No | Content returned as-is |

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

## Skill Files

Skills can include supporting files (scripts, reference docs). When present, Bridge writes them to `.skills/<skill-id>/` on the filesystem so the agent can execute scripts directly.

When a skill with files is invoked, the output includes a location note:

```
NOTE: This skill's files are at .skills/use-railway/
Prefix script paths with this directory.

---

[skill content]
```

The `${CLAUDE_SKILL_DIR}` variable in skill content is substituted with the filesystem path (e.g., `.skills/use-railway`).

Scripts with `.sh`, `.py`, or `.rb` extensions are automatically marked executable. Files are cleaned up when an agent is removed or updated.

See [Skill Files](../core-concepts/skills.md#skill-files) for full details.

---

## Implementation Details

### Skill File Storage

When a skill has files, they are written to `.skills/<skill-id>/` relative to the working directory at agent load time. The agent can execute scripts by path. Files are cleaned up on agent removal or update.

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
