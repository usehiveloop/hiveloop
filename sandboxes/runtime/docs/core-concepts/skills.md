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
| `description` | What the skill does (shown to the AI in system reminders) |
| `content` | The actual prompt content (only revealed when skill is invoked) |
| `files` | Supporting files map (`relative_path` → `content`). Written to disk at `.skills/<id>/`. See [Skill Files](#skill-files). |
| `parameters_schema` | Reserved for future structured parameters support |

---

## How Skills Appear in System Prompts

When an agent has skills defined, Bridge injects a **system reminder** before every user message:

```xml
<system-reminder>

# System Reminders

## Available skills

The following skills are available for use with the Skill tool:

- **Write Commit Message** - Write clear, conventional commit messages from git diffs
- **Code Review** - Review code for bugs, security issues, and style

</system-reminder>
```

**Important:** The system reminder only shows the **title** and **description** of each skill. The actual `content` remains hidden until the skill is explicitly invoked. This helps the AI know what skills are available without consuming context window tokens for skill content that may not be needed.

---

## Using Skills

Agents use the `skill` tool to invoke a skill. This is a **blocking requirement** — when a skill matches the user's request, the agent must invoke the skill tool BEFORE generating any other response.

```json
{
  "name": "skill",
  "arguments": {
    "name": "commit",
    "args": "fix: handle null pointer in user authentication"
  }
}
```

The skill tool loads the skill content and returns it wrapped in XML tags:

```xml
<skill_content name="commit" title="Write Commit Message">
You are an expert at writing commit messages. Given a git diff, write a commit message that follows the Conventional Commits format. Be concise but descriptive.
</skill_content>
```

---

## Skill Matching

The skill tool matches by **ID or title** (case-insensitive):

| Query | Matches skill with... |
|-------|----------------------|
| `"commit"` | id: `commit` or title: `Commit` |
| `"Code Review"` | id: `code-review` or title: `Code Review` |
| `"PR-SUMMARY"` | id: `pr-summary` or title: `PR Summary` |

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

If a skill doesn't have `{{args}}`, the args are appended with an "Arguments:" prefix:

```json
{
  "id": "review",
  "content": "You are a code reviewer."
}
```

With args `"Review this function"`, the content becomes:

```
You are a code reviewer.

Arguments: Review this function
```

### Without Args

If no args are provided, the skill content is returned unchanged (including any `{{args}}` placeholders).

---

## Error Handling

### Skill Not Found

If an agent tries to use a skill that doesn't exist, the tool returns an error:

```
Skill 'unknown-skill' not found. Available skills: [Code Review, Commit]
```

The error includes the list of available skill titles to help identify the correct skill name.

### No Skills Defined

The skill tool is **only registered if the agent has at least one skill defined**. Agents without skills cannot use this tool.

---

## Maximum Number of Skills

There is **no enforced maximum** number of skills per agent. However:

- All skill titles and descriptions appear in every system reminder
- A very large number of skills will consume significant context window tokens
- Keep the list focused on skills the agent actually needs

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
  "title": "Refactor Code",
  "description": "Refactor code while preserving behavior",
  "content": "You are an expert at refactoring. When refactoring code: 1) Preserve existing behavior, 2) Improve readability, 3) Add comments for complex logic, 4) Keep functions under 50 lines. Explain your changes."
}
```

---

## Skill Discovery

The AI knows about available skills through the system reminder that appears before every user message. The reminder includes:

- The skill title (for matching when the user requests it)
- The skill description (to help the AI understand when to use it)

Additionally, the skill tool's description includes instructions on how to invoke skills and handle "slash command" syntax (e.g., `/commit`, `/review`).

---

## Important Behavior Notes

### Blocking Requirement

When a skill matches the user's request, invoking the skill tool is a **blocking requirement**. The agent must call the skill tool BEFORE generating any text response about the task.

### Skill Content is Hidden Until Invoked

The actual skill content is **not** shown in the system reminder. It is only revealed when the skill tool is invoked. This:

- Keeps the context window clean
- Prevents the AI from "pretending" to use a skill without actually calling the tool
- Allows skills to be large without impacting every conversation turn

### Already Running Detection

The skill tool instructions warn agents not to invoke a skill that is already running. If the conversation already contains a `<skill_content>` tag from a previous invocation in the current turn, the agent should follow those instructions directly instead of calling the tool again.

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

## Skill Files

Skills can include supporting files — scripts, reference docs, and other assets. When a skill has files, Bridge writes them to disk so the agent can execute scripts directly.

### Defining Files

Files are defined as a map of relative paths to content in your skill definition:

```json
{
  "id": "use-railway",
  "title": "Use Railway",
  "description": "Operate Railway infrastructure",
  "content": "# Use Railway\n\nRun scripts/railway-api.sh to query the Railway API...",
  "files": {
    "scripts/railway-api.sh": "#!/usr/bin/env bash\nset -e\n...",
    "scripts/analyze-postgres.py": "#!/usr/bin/env python3\nimport json\n...",
    "scripts/dal.py": "#!/usr/bin/env python3\n...",
    "references/deploy.md": "# Deploy guide\n..."
  }
}
```

### Filesystem Layout

When a skill with files is loaded, Bridge writes them to `.skills/<skill-id>/` relative to the working directory:

```
.skills/use-railway/
├── scripts/
│   ├── railway-api.sh      (executable)
│   ├── analyze-postgres.py  (executable)
│   └── dal.py               (executable)
└── references/
    └── deploy.md
```

Scripts with `.sh`, `.py`, or `.rb` extensions are automatically marked executable.

### How the Agent Uses Skill Files

When the skill tool is invoked, Bridge prepends a note telling the agent where files live:

```
NOTE: This skill's files are at .skills/use-railway/
Prefix script paths with this directory.

---

# Use Railway
...
```

The agent then runs scripts directly:

```bash
.skills/use-railway/scripts/railway-api.sh '{"query": "..."}'
```

Python scripts with relative imports work because sibling files exist on disk:

```bash
cd .skills/use-railway/scripts && python3 analyze-postgres.py --service-id abc123
```

### The `file` Parameter

The agent can also request a specific file without loading the full skill content:

```json
{
  "name": "skill",
  "arguments": {
    "name": "use-railway",
    "file": "references/deploy.md"
  }
}
```

This returns only that file's content, useful for quick lookups.

### Cleanup

When an agent is removed or updated with different skills, Bridge removes the old `.skills/<skill-id>/` directory automatically.

### The `${CLAUDE_SKILL_DIR}` Variable

Skill content can reference `${CLAUDE_SKILL_DIR}`, which Bridge substitutes with the skill's filesystem path (e.g., `.skills/use-railway`):

```
Run ${CLAUDE_SKILL_DIR}/scripts/deploy.sh
```

Becomes:

```
Run .skills/use-railway/scripts/deploy.sh
```

---

## Future: Structured Parameters

The `parameters_schema` field in skill definitions is reserved for future support of structured parameters (JSON Schema). Currently, skills only support the simple `args` string parameter.

For now, use the `args` string parameter and `{{args}}` template substitution.

---

## See Also

- [Skill Tool](../tools-reference/skill-tool.md) — Tool reference
- [Tools](tools.md) — Tool system overview
- [Agents](agents.md) — Agent configuration
