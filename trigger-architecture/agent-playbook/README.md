# Agent Playbook

This playbook is for people **using** the trigger system to build autonomous agents, not the people building the trigger system itself. If you want to know how the dispatcher pipeline works internally, read the [top-level architecture docs](../README.md). If you want to know how to design an agent that reliably does useful work, read these.

The playbook covers strategy and tradeoffs rather than code. When you finish reading it, you should know:

- How to decide what an agent should do (and what it shouldn't)
- How to pick triggers, conditions, and context without shooting yourself in the foot
- How to think about lifecycle: when to keep a conversation going vs. start a new one
- How to prevent the failure modes that bite everyone who doesn't know about them (infinite loops, cost blowouts, agents talking past each other)
- How to know whether your configuration is actually working in production
- What the system genuinely can't do yet, and how to work around those gaps

## Who this is for

- **Founding engineers** setting up their first autonomous agent and picking a shape that won't paint them into a corner
- **Platform/automation teams** rolling out agent-based workflows to multiple internal customers
- **PMs and tech leads** evaluating what the system can realistically do for a specific use case
- **Anyone debugging** a trigger configuration that isn't behaving the way they expected

The playbook assumes you've read, or can read, the top-level overview in [../README.md](../README.md). It doesn't assume you know the Go code.

## Reading order

If you're just starting out, read these in order:

1. [01-agent-design-principles.md](01-agent-design-principles.md) — how to think about what an agent is and isn't
2. [02-trigger-configuration-strategy.md](02-trigger-configuration-strategy.md) — picking events, writing conditions, choosing context
3. [04-safety-and-loop-prevention.md](04-safety-and-loop-prevention.md) — the failure mode everyone hits; read this before writing your first config
4. [07-worked-examples.md](07-worked-examples.md) — full YAML for real-world setups, with commentary

If you already have an agent running and it's misbehaving:

- [06-observability-and-debugging.md](06-observability-and-debugging.md) — how to figure out what went wrong
- [08-anti-patterns.md](08-anti-patterns.md) — maybe you're doing one of these

If you're planning a rollout or capacity-planning:

- [05-economics-and-performance.md](05-economics-and-performance.md) — cost structure and tradeoffs
- [09-practical-limitations.md](09-practical-limitations.md) — what to plan around

## Index

| File | What's in it |
|---|---|
| [01-agent-design-principles.md](01-agent-design-principles.md) | What makes a good agent. Single responsibility, scope boundaries, when to create vs. extend an existing agent. |
| [02-trigger-configuration-strategy.md](02-trigger-configuration-strategy.md) | How to pick triggers, write conditions, choose context actions, and write instructions that guide without over-specifying. |
| [03-lifecycle-and-state-management.md](03-lifecycle-and-state-management.md) | When conversations continue, when they end, how to think about long-running agent work, and how memory fits in. |
| [04-safety-and-loop-prevention.md](04-safety-and-loop-prevention.md) | The biggest risk in the system: agents triggering themselves. Self-exclusion, write-action discipline, circuit breakers. |
| [05-economics-and-performance.md](05-economics-and-performance.md) | Per-trigger cost structure, sandbox economics, context fetch budgets, scaling considerations. |
| [06-observability-and-debugging.md](06-observability-and-debugging.md) | How to know a trigger is working, how to read dispatch logs, how to debug skipped runs. |
| [07-worked-examples.md](07-worked-examples.md) | End-to-end YAML for realistic agents: PR reviewer, bug triager, CI responder, autonomous coder, support drafter. With commentary on each choice. |
| [08-anti-patterns.md](08-anti-patterns.md) | Things that look sensible but cause problems, with better alternatives. |
| [09-practical-limitations.md](09-practical-limitations.md) | Business-level constraints you need to know about when planning a rollout, and workarounds where applicable. |

## One-sentence summary

The trigger system is a powerful tool for building agents that react to real-world events, but it rewards users who think carefully about scope, safety, and lifecycle before writing their first line of YAML — this playbook is the shortcut to doing that well.

## When to update this playbook

- A customer hits a pitfall that isn't documented here → add to [08-anti-patterns.md](08-anti-patterns.md)
- A new best practice emerges from production use → add to the relevant strategy doc
- A limitation gets lifted → update [09-practical-limitations.md](09-practical-limitations.md) and note the date
- A new common use case emerges → add a worked example to [07-worked-examples.md](07-worked-examples.md)

The playbook is living documentation. The technical architecture docs change when code changes; this changes when our understanding of how to use the system changes.
