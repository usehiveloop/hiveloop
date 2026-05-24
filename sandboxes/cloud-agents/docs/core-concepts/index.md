# Core Concepts

This section explains how Bridge works. Read this to understand the mental model before building your integration.

---

## The Big Picture

Bridge sits between your control plane and AI providers:

```
Your Backend          Bridge              AI Providers
     │                  │                       │
     │ Push agents      │                       │
     ├─────────────────►│                       │
     │                  │                       │
     │                  │ API calls             │
     │                  ├──────────────────────►│
     │                  │                       │
     │                  │ Streaming response    │
     │                  │◄──────────────────────┤
     │                  │                       │
     │ Webhook events   │                       │
     │◄─────────────────┤                       │
```

Your control plane owns agent definitions. Bridge runs them.

---

## Key Concepts

### [Agents](agents.md)
An agent is a complete AI configuration: the model it uses, what tools it has access to, and the instructions it follows. You define agents, push them to Bridge, and they stay ready to handle conversations.

### [Conversations](conversations.md)
A conversation is a single chat session between a user and an agent. Bridge manages the state, history, and streaming of each conversation.

### [Tools](tools.md)
Tools are things an agent can do. Read files, run commands, search the web — each is a tool. You configure which tools an agent can use when you define it.

### [MCP](mcp.md)
MCP (Model Context Protocol) lets Bridge connect to external tool servers. It's a standard way to add capabilities like filesystem access or database queries.

### [Skills](skills.md)
Skills are reusable prompt templates. An agent can pull in a skill when it needs specific expertise, like "write a good commit message" or "review code."

### [System Reminders](system-reminders.md)
System reminders are markdown blocks injected before every user message. They provide runtime context like available skills, subagents, the current date, and todo state — without modifying the system prompt.

### [Webhooks](webhooks.md)
Webhooks are how Bridge talks back to your control plane. When things happen — messages sent, tools called, conversations ending — Bridge sends events to your webhook URL.

---

## Read in This Order

1. **[Architecture](architecture.md)** — How Bridge is built internally
2. **[Agents](agents.md)** — Agent lifecycle and configuration
3. **[Conversations](conversations.md)** — How conversations work
4. **[Tools](tools.md)** — The tool system
5. **MCP, Skills, Webhooks** — As needed for your use case

---

## Common Patterns

After reading this section, you'll understand:

- How to design agents for your use case
- When to use tools vs MCP servers vs skills
- How conversation state flows through the system
- How to handle webhooks reliably

Ready? Start with [Architecture](architecture.md).
