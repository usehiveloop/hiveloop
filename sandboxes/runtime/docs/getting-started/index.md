# Getting Started

Welcome to Bridge. This section will take you from zero to running your first AI agent in about 15 minutes.

---

## What You'll Learn

By the end of this section, you will:

1. **Install Bridge** — Build from source using Rust
2. **Run your first agent** — Create a simple agent and talk to it
3. **Understand configuration** — Learn how to configure Bridge with environment variables
4. **Know your deployment options** — Run with Docker or as a binary

---

## Prerequisites

Before you start, make sure you have:

- **Rust** — The stable toolchain (install from [rustup.rs](https://rustup.rs))
- **Git** — To clone the repository
- **An API key** — From an AI provider (Anthropic, OpenAI, or others)

That's it. No databases to set up, no cloud accounts to create (except for the AI provider).

---

## The Five-Minute Pitch

Bridge is an AI runtime. Here's what that means:

- You define **agents** — AI workers with specific instructions and tools
- You **push** those definitions to Bridge
- Bridge **runs** conversations with those agents
- Your users talk to the agents through your frontend
- Bridge **streams** responses in real time

Bridge handles the messy parts: managing conversation state, calling tools, talking to AI providers, and streaming results back to your users.

---

## Next Steps

Ready? Start with [Installation](installation.md).

If you want to skip ahead:

- Already have Rust? Go to [Quickstart](quickstart.md)
- Want to use Docker? Jump to [Docker](docker.md)
- Need the full config reference? See [Configuration](configuration.md)
