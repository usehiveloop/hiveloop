# Bridge Documentation

> Run AI agents with tools, integrations, and real-time streaming.

---

## What is Bridge?

Bridge is an AI runtime that runs your agents. You define what your agents can do, push those definitions to Bridge, and it handles the rest: running conversations, calling tools, managing state, and streaming responses in real time.

Think of it as the engine that powers your AI features. Your backend (the "control plane") owns the agent definitions and conversation history. Bridge runs the agents and talks back to you via webhooks.

---

## Quick Navigation

### 🚀 New to Bridge?
Start here to get up and running:

- [Installation](getting-started/installation.md) — Install Rust and build Bridge
- [Quickstart](getting-started/quickstart.md) — Run your first agent in 5 minutes
- [Configuration](getting-started/configuration.md) — Set up environment variables

### 🧠 Want to understand how it works?
Learn the core concepts:

- [Architecture](core-concepts/architecture.md) — How Bridge is built
- [Agents](core-concepts/agents.md) — Agent lifecycle and configuration
- [Conversations](core-concepts/conversations.md) — How conversations work
- [Tools](core-concepts/tools.md) — The tool system

### 📡 Building an integration?
Connect your control plane:

- [Push API](api-reference/push-api.md) — Send agents to Bridge
- [Handling Webhooks](control-plane/handling-webhooks.md) — Receive events from Bridge
- [Hydrating Conversations](control-plane/hydrating-conversations.md) — Restore conversation history

### 🛠️ Looking for tool details?
Browse the tool reference:

- [Filesystem Tools](tools-reference/filesystem-tools.md) — read, write, edit files
- [Agent Tools](tools-reference/agent-tools.md) — spawn_agent, parallel_agent, join
- [All Tools](tools-reference/index.md) — Complete list

### 🚢 Deploying to production?

- [Binary Deployment](deployment/binary-deployment.md)
- [Docker Deployment](deployment/docker-deployment.md)
- [Kubernetes](deployment/kubernetes.md)

---

## Architecture at a Glance

```
┌─────────────────┐     Push agents & history      ┌─────────────┐
│  Control Plane  │ ─────────────────────────────► │    Bridge   │
│   (your code)   │                                │  (runtime)  │
└─────────────────┘◄───────────────────────────────┴─────────────┘
        │                  Webhooks (events)
        │
        ▼
┌─────────────────┐
│    Frontend     │ ◄────────────────────────────── SSE stream
│  (your web UI)  │                                (real-time)
└─────────────────┘
```

---

## Getting Help

- **Questions?** Open a GitHub discussion
- **Bugs?** File an issue with reproduction steps
- **Contributing?** See the [Development Guide](development/index.md)

---

## License

MIT License — see the repository for details.
