# System Reminders

System reminders are markdown blocks injected before every user message in a conversation. They provide runtime context to the agent without modifying the system prompt.

---

## How They Work

```
User sends message
       ↓
Bridge builds system reminder (skills, subagents, date, todos)
       ↓
Reminder is prepended to the user message
       ↓
Agent sees: [system reminder] + [user message]
```

System reminders are wrapped in `<system-reminder>` tags so the agent can distinguish them from user content.

---

## Format

Every system reminder follows this structure:

```xml
<system-reminder>

# System Reminders

## Section Title

Section content...

## Another Section

More content...

</system-reminder>
```

Sections are only included when they have content. An agent with no skills, no subagents, and no todos will still receive a Current Date section.

---

## Sections

### Available Skills

Listed when the agent has at least one skill defined. Shows skill titles and descriptions so the agent knows what skills it can invoke, without revealing the full skill content.

```
## Available skills

The following skills are available for use with the Skill tool:

- **Code Review** - Reviews code for quality and best practices
- **Commit** - Writes conventional commit messages
```

If the agent has no skills, this section is omitted entirely.

### Available Subagents

Listed when the agent has at least one subagent defined. Shows subagent names and descriptions to help the agent decide which subagent to delegate work to.

```
## Available sub-agents

The following sub-agents are available for use with the Agent tool:

- **researcher** - Searches and summarizes information
- **coder** - Writes and reviews code
```

If the agent has no subagents, this section is omitted entirely.

### Current Date

Always included. Shows today's date in a human-readable format.

```
## Current date

Today is Saturday, April 05, 2026.
```

### Todo List

Listed when the conversation has active todo items. Shows each task with its status and priority, plus a count of incomplete tasks.

```
## Todo List

You have 2 task(s) in progress.

1. [high] [in_progress] Implement user authentication
2. [medium] [pending] Write unit tests for auth module
3.  [completed] Set up project structure

**Important**: Please update your progress with todos as soon as there's an update, rather than waiting until the end.
```

If there are no todos, this section is omitted. If all todos are completed or cancelled, the section shows "All tasks are complete!" instead of the in-progress count.

---

## Date Tracking

Bridge uses a `DateTracker` to detect when the date changes between conversation turns. This handles long-running conversations that span midnight or multiple days.

When the date changes, Bridge injects a separate date change notification:

**Single day change:**

```xml
<system-reminder>

# Date Change

Note: The date has changed. It is now April 06, 2026 (was April 05, 2026).

</system-reminder>
```

**Multi-day change:**

```xml
<system-reminder>

# Date Change

Note: The date has changed by 3 days. It is now April 08, 2026 (was April 05, 2026).

</system-reminder>
```

The date change notification is a separate `<system-reminder>` block from the main system reminder. After the change is detected, the tracker updates its internal state so the notification is only sent once per date transition.

---

## Full Example

An agent with two skills, two subagents, one active todo, and today's date would produce:

```xml
<system-reminder>

# System Reminders

## Available skills

The following skills are available for use with the Skill tool:

- **Code Review** - Reviews code for quality and best practices
- **Commit** - Writes conventional commit messages

## Available sub-agents

The following sub-agents are available for use with the Agent tool:

- **researcher** - Searches and summarizes information
- **coder** - Writes and reviews code

## Todo List

You have 1 task(s) in progress.

1. [high] [in_progress] Implement user authentication

**Important**: Please update your progress with todos as soon as there's an update, rather than waiting until the end.

## Current date

Today is Saturday, April 05, 2026.

</system-reminder>
```

---

## System Reminders vs System Prompt

| | System Prompt | System Reminders |
|---|---------------|------------------|
| When applied | Once, at conversation start | Before every user message |
| Purpose | Core agent instructions and personality | Runtime context (date, available tools, task state) |
| Content | Static (defined in agent config) | Dynamic (changes between turns) |
| Who controls it | You (agent definition) | Bridge (automatically generated) |

The system prompt is the agent's "job description." System reminders are the "daily briefing" that keeps the agent aware of what it can do and what state the conversation is in.

---

## When Sections Are Included

| Section | Condition |
|---------|-----------|
| Available Skills | Agent has at least one skill defined |
| Available Subagents | Agent has at least one subagent defined |
| Todo List | Conversation has at least one todo item |
| Current Date | Always included |

If all conditional sections are empty (no skills, no subagents, no todos), the reminder still contains the Current Date section.

---

## See Also

- [Skills](skills.md) -- Skill definitions and usage
- [Agents](agents.md) -- Agent configuration
- [Agent Tools](../tools-reference/agent-tools.md) -- Subagent tools
