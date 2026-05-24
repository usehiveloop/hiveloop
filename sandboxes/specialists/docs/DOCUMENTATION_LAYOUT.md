# Bridge Documentation Layout

> Proposed structure for comprehensive Bridge documentation

---

## 📁 Structure Overview

```
docs/
├── README.md                     # Documentation homepage & navigation
├── SUMMARY.md                    # Table of contents (for mdBook/Docusaurus)
├── 
├── getting-started/              # 🚀 Getting Started (5 chapters)
│   ├── index.md                  #   GS landing: "What you'll learn"
│   ├── installation.md           #   Install Rust, build from source
│   ├── quickstart.md             #   First agent running in 5 minutes
│   ├── configuration.md          #   env vars, config.toml reference
│   └── docker.md                 #   Docker deployment
│
├── core-concepts/                # 🧠 Core Concepts (8 chapters)
│   ├── index.md                  #   Mental model & architecture overview
│   ├── architecture.md           #   System architecture diagram
│   ├── agents.md                 #   Agent lifecycle, versions, draining
│   ├── conversations.md          #   Conversation state, turns, streaming
│   ├── tools.md                  #   Built-in tools, tool execution
│   ├── mcp.md                    #   MCP servers & integration
│   ├── skills.md                 #   Skills system & parameter substitution
│   └── webhooks.md               #   Webhook events & HMAC signing
│
├── api-reference/                # 📡 API Reference (6 chapters)
│   ├── index.md                  #   API overview & authentication
│   ├── authentication.md         #   Bearer tokens, API keys
│   ├── agents-api.md             #   /agents/* endpoints
│   ├── conversations-api.md      #   /conversations/* endpoints
│   ├── push-api.md               #   /push/* control plane endpoints
│   └── sse-events.md             #   Server-Sent Events reference
│
├── control-plane/                # 🎛️ Control Plane Integration (5 chapters)
│   ├── index.md                  #   Push-based architecture explained
│   ├── pushing-agents.md         #   Agent definition format
│   ├── hydrating-conversations.md #   Conversation history hydration
│   ├── handling-webhooks.md      #   Receiving & verifying webhooks
│   └── diff-updates.md           #   Incremental agent updates
│
├── llm-providers/                # 🤖 LLM Providers (4 chapters)
│   ├── index.md                  #   Provider types: native vs openai-compat
│   ├── anthropic.md              #   Claude setup & models
│   ├── openai-compatible.md      #   Groq, DeepSeek, Mistral, etc.
│   └── custom-providers.md       #   Bringing your own provider
│
├── tools-reference/              # 🛠️ Tools Reference (12 chapters)
│   ├── index.md                  #   Tool system overview
│   ├── filesystem-tools.md       #   read, write, edit, ls, glob
│   ├── bash-tool.md              #   Shell command execution
│   ├── search-tools.md           #   grep, search
│   ├── web-tools.md              #   web_fetch, web_search
│   ├── todo-tool.md              #   Task management
│   ├── batch-tool.md             #   Batch operations
│   ├── agent-tools.md            #   agent, sub_agent (with runInBackground)
│   ├── skill-tool.md             #   skill tool & template substitution
│   ├── lsp-tools.md              #   Code intelligence tools
│   └── custom-tools.md           #   Creating custom tools
│
├── deployment/                   # 🚢 Deployment & Operations (5 chapters)
│   ├── index.md                  #   Production checklist
│   ├── binary-deployment.md      #   Running the binary
│   ├── docker-deployment.md      #   Docker & Compose
│   ├── kubernetes.md             #   K8s manifests & Helm
│   └── monitoring.md             #   Metrics, health checks, logging
│
├── tutorials/                    # 📚 Step-by-Step Tutorials (4 tutorials)
│   ├── index.md                  #   Tutorial index
│   ├── customer-support-agent.md #   Build a support agent
│   ├── code-review-agent.md      #   Build a code review agent
│   ├── multi-agent-system.md     #   Parent + subagent pattern
│   └── github-integration.md     #   GitHub PR creation workflow
│
├── development/                  # 🔧 Development (4 chapters)
│   ├── index.md                  #   Contributing guide
│   ├── architecture-deep-dive.md #   Crate-by-crate architecture
│   ├── testing.md                #   Unit & E2E testing guide
│   └── adding-a-tool.md          #   Tutorial: implement new tool
│
└── reference/                    # 📖 Reference Materials
    ├── glossary.md               #   Terminology definitions
    ├── environment-variables.md  #   Complete env var reference
    ├── openapi.md                #   Link to openapi.json
    └── changelog.md              #   Release notes
```

---

## 📊 Page Count Estimate

| Section | Pages | Purpose |
|---------|-------|---------|
| Getting Started | 5 | Onboard new users quickly |
| Core Concepts | 8 | Build mental model |
| API Reference | 6 | Day-to-day API lookup |
| Control Plane | 5 | Integration guide for backend devs |
| LLM Providers | 4 | Configuration reference |
| Tools Reference | 11 | Tool usage & parameters |
| Deployment | 5 | Operations & DevOps |
| Tutorials | 4 | Hands-on learning |
| Development | 4 | Contributors & extenders |
| Reference | 4 | Quick lookups |
| **TOTAL** | **~56 pages** | Complete coverage |

---

## 🎯 Documentation Goals

1. **New Developer**: Can go from zero to running first agent in 15 minutes
2. **Backend Engineer**: Can integrate control plane push/hydrate/webhooks in 2 hours
3. **DevOps**: Can deploy to production with confidence
4. **Contributor**: Can understand architecture and add features
5. **Day-to-Day User**: Can quickly find API details and tool syntax

---

## ❓ Questions for You

1. **Platform preference**: Static site (mdBook), Docusaurus, or plain Markdown?
2. **Hosting**: GitHub Pages, Vercel, or other?
3. **Versioning**: Document multiple versions or single latest?
4. **Code examples**: Rust only, or include Node.js/Python control plane examples?
5. **Priority**: Which sections are most urgent for your users?

---

*Ready for your review. Once layout is approved, I'll propose tone of voice and documentation strategy.*
