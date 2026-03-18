# Skills

Skills are reusable prompt templates. An agent can pull in a skill when it needs specific expertise.

---

## What are Skills?

A skill is a pre-written prompt for a specific task. Instead of putting everything in the system prompt, agents can invoke skills as needed.

```
Agent: "Write a commit message for these changes"
       ↓
Uses "commit" skill
       ↓
Skill content: "You are a commit message writer...
                Analyze the diff and write a clear,
                concise commit message..."
       ↓
Agent applies the skill, writes the commit message
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
      "description": "Write clear, conventional commit messages from git diffs",
      "content": "You are an expert at writing commit messages. Given a git diff, write a commit message that follows the Conventional Commits format. Be concise but descriptive."
    },
    {
      "id": "review",
      "title": "Code Review",
      "description": "Review code for bugs, security issues, and style",
      "content": "You are a senior engineer doing code review. Check for: 1) Security issues, 2) Logic errors, 3) Performance problems, 4) Style consistency. Provide specific, actionable feedback."
    }
  ]
}
```

### Skill Fields

| Field | Purpose |
|-------|---------|
| `id` | Unique identifier (used when invoking) |
| `title` | Human-readable name |
| `description` | What the skill does (shown to the AI) |
| `content` | The actual prompt content |

---

## Using Skills

Agents use the `skill` tool to invoke a skill:

```json
{
  "name": "skill",
  "arguments": {
    "name": "commit",
    "args": "fix: handle null pointer in user authentication"
  }
}
```

The `skill` tool loads the skill content and makes it available to the agent.

---

## Parameter Substitution

Skills can include placeholders for dynamic content:

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
    "args": "Fixed the login bug where users couldn't sign in with email"
  }
}
```

The skill content becomes:

```
Write a commit message for these changes: Fixed the login bug where users couldn't sign in with email
```

### Skills Without Placeholders

If a skill doesn't have `{{args}}`, the args are appended:

```json
{
  "id": "review",
  "content": "You are a code reviewer."
}
```

With args `"Review this function"`, the content becomes:

```
You are a code reviewer.

Additional context: Review this function
```

---

## When to Use Skills

Skills are useful when:

| Scenario | Example |
|----------|---------|
| Multiple tasks, one agent | An engineer agent that can code, review, and write commits |
| Large prompts | Breaking up a long system prompt into focused skills |
| Reusable expertise | Same "code review" skill used across multiple agents |
| User-triggered modes | User clicks "Review my code" vs "Write tests" |

---

## Skills vs System Prompt

| | System Prompt | Skills |
|---|---------------|--------|
| **When applied** | Every message | Only when invoked |
| **Size** | Keep concise | Can be detailed |
| **Flexibility** | Fixed for conversation | Switch as needed |
| **Use for** | Core behavior, personality | Specific tasks |

### Good System Prompt

```json
{
  "system_prompt": "You are a helpful coding assistant. You can write code, review code, and explain concepts. Use tools when needed. Be concise."
}
```

### Good Skill

```json
{
  "id": "refactor",
  "content": "You are an expert at refactoring. When refactoring code: 1) Preserve existing behavior, 2) Improve readability, 3) Add comments for complex logic, 4) Keep functions under 50 lines. Explain your changes."
}
```

---

## Skill Discovery

The AI knows about available skills through tool descriptions. When you define skills, Bridge automatically creates descriptions for the `skill` tool:

```
skill(name: string, args?: string)

Available skills:
- commit: Write clear, conventional commit messages
- review: Review code for bugs and security issues  
- refactor: Improve code structure and readability
```

The AI uses these descriptions to decide which skill to invoke.

---

## Advanced: Skill Parameters (Future)

Skills will support structured parameters in a future release:

```json
{
  "id": "commit",
  "parameters": {
    "type": "object",
    "properties": {
      "type": { "enum": ["feat", "fix", "docs", "style"] },
      "scope": { "type": "string" },
      "breaking": { "type": "boolean" }
    }
  },
  "content": "Write a {{type}} commit..."
}
```

For now, use the `args` string parameter.

---

## Example: Multi-Mode Agent

Here's an agent with skills for different modes:

```json
{
  "id": "senior-engineer",
  "name": "Senior Engineer",
  "system_prompt": "You are a senior software engineer. Help the user with coding tasks. Use skills for specialized work.",
  "skills": [
    {
      "id": "code",
      "title": "Write Code",
      "description": "Write clean, well-tested code",
      "content": "Write code following the project's style. Include error handling. Prefer clarity over cleverness."
    },
    {
      "id": "review",
      "title": "Code Review", 
      "description": "Review code thoroughly",
      "content": "Review this code carefully. Check: security, correctness, performance, maintainability. List issues by severity."
    },
    {
      "id": "explain",
      "title": "Explain Code",
      "description": "Explain how code works",
      "content": "Explain this code in simple terms. Start with the big picture, then dive into details. Use analogies where helpful."
    }
  ],
  "tools": ["read", "write", "edit", "bash"]
}
```

The agent can switch modes by using different skills.

---

## See Also

- [Skill Tool](../tools-reference/skill-tool.md) — Tool reference
- [Tools](tools.md) — Tool system overview
- [Agents](agents.md) — Agent configuration
