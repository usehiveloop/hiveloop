# Glossary

Terms used in Bridge.

---

## A

**Agent** — An AI configuration with a system prompt, tools, and settings. The thing users talk to.

**API Key** — Secret token for authenticating with Bridge's push endpoints.

## B

**Bridge** — The AI runtime that runs agents. This software.

## C

**Compaction** — Summarizing old conversation history to save tokens.

**Control Plane** — Your backend that owns agent definitions and handles webhooks.

**Conversation** — A single chat session between a user and an agent.

## D

**Draining** — Gracefully shutting down an agent by letting active conversations finish.

## E

**Event** — Something that happened (message sent, tool called, etc.). Sent via webhooks.

## H

**Hydration** — Restoring a conversation's message history from storage.

## I

**Integration** — An HTTP endpoint Bridge calls as a tool.

## L

**LLM** — Large Language Model (the AI provider).

**LSP** — Language Server Protocol. For code intelligence.

## M

**MCP** — Model Context Protocol. Standard for external tool servers.

**Message** — A single item in a conversation (user or assistant).

## P

**Permission** — Control over tool execution (allow, require_approval, deny).

**Provider** — An AI service (Anthropic, OpenAI, etc.).

**Push** — Sending agent definitions to Bridge.

## S

**Skill** — Reusable prompt template.

**SSE** — Server-Sent Events. Real-time streaming protocol.

**Subagent** — An agent that another agent can spawn.

## T

**Tool** — Something an agent can do (read file, run command, etc.).

**Turn** — One back-and-forth exchange in a conversation.

## V

**Version** — String identifier for an agent definition. Changing it triggers updates.

## W

**Webhook** — HTTP callback Bridge sends to your control plane.

---

## See Also

- [Core Concepts](../core-concepts/index.md)
