# Bridge Documentation: Tone of Voice & Writing Strategy

> A senior technical writer's proposal for Bridge documentation voice, style, and approach.

---

## 🎯 Voice Character

Bridge speaks as a **knowledgeable colleague** — the engineer at the desk next to you who actually explains things well. Plain-spoken, practical, and allergic to corporate nonsense.

| Trait | We Are | We Are NOT |
|-------|--------|------------|
| **Expertise** | Experienced but approachable | Arrogant, condescending, showy |
| **Clarity** | Plain English, no jargon unless necessary | Academic, buzzword-heavy, "leverage synergy" |
| **Helpfulness** | Anticipating questions, offering guidance | Reactive, minimal, RTFM attitude |
| **Personality** | Professional with warmth, conversational | Robotic, overly casual, or corporate-speak |
| **Efficiency** | Information-dense, scannable | Wall-of-text, repetitive, fluffy |

---

## ✍️ Writing Principles

### 1. Lead with the "Why" in Plain English
Before showing code, explain what problem this solves. Use words you'd use when explaining to a coworker over coffee.

**❌ Weak (jargony):**
> The `/push/agents` endpoint accepts agent definitions for provisioning within the runtime environment.

**❌ Corporate-speak:**
> Bridge enables seamless agent onboarding via our best-in-class provisioning API, empowering developers to leverage AI at scale.

**✅ Strong:**
> Bridge starts empty. You have to tell it about your agents first. Send them to `/push/agents` and they'll be ready to use right away.

---

### 2. Show, Don't Just Tell
Every concept gets a concrete example. Every endpoint gets a curl command + response.

**Pattern:**
1. Brief explanation (1-2 sentences)
2. Working example
3. Explanation of key parts

---

### 3. Use Plain Language
Avoid jargon. When you need a technical term, introduce it naturally.

**❌ Jargony:**
> The agent runtime implements a push-based provisioning model with eventual consistency for state hydration.

**✅ Plain:**
> You push agent configs to Bridge when they change. Bridge updates its copy immediately — there's no polling or waiting.

**When technical terms are needed:**
> Bridge uses **MCP** (Model Context Protocol) to talk to external tools. Think of it as a standard way for AI systems to connect to things like file browsers or databases.

---

### 4. Progressive Disclosure
Structure content for different depths:

- **First paragraph:** The 30-second version
- **First section:** The 5-minute practical guide
- **Later sections:** Deep dives for advanced use cases
- **Sidebars/notes:** Edge cases, internals, "why we did it this way"

---

### 5. Active Voice, Present Tense

**❌ Weak:**
> The agent definition will be validated by the server.

**✅ Strong:**
> The server checks the agent definition.

---

### 6. Address the Reader Directly
Use "you" and imperative verbs for instructions. Write like you're talking to one person.

**❌ Corporate:**
> Developers are advised to configure webhook URLs prior to production deployment.

**❌ Indirect:**
> The webhook URL should be configured...

**✅ Direct:**
> Set your webhook URL before deploying to production.

---

## 📝 Language Standards

### No Corporate Speak
These words are banned unless they have specific technical meanings:

| ❌ Banned | ✅ Use Instead |
|-----------|----------------|
| leverage | use |
| utilize | use |
| facilitate | help, make easier |
| utilize | use |
| empower | let you, help you |
| seamless | easy, automatic |
| best-in-class | (just describe what it does) |
| scalable | (explain what happens when load increases) |
| robust | (explain what makes it reliable) |
| solution | tool, system, or (just name the thing) |
| deliver | give, send, provide |
| drive | cause, make happen |
| enable | let, allow, make possible |

### Terminology Consistency
Use these terms consistently. But when introducing them, explain them in plain English.

| Term | Plain English Explanation | Avoid |
|------|---------------------------|-------|
| **Control plane** | Your backend — the system you build that tells Bridge what to do | "Backend," "server" (too vague) |
| **Bridge** | This software — the AI runtime you're reading about | "bridge," "the bridge" (lowercase) |
| **Agent** | An AI worker with specific instructions and tools | "Bot," "assistant" |
| **Conversation** | One ongoing chat between a user and an agent | "Chat," "thread" |
| **Tool** | Something an agent can do (like "read a file" or "search the web") | "Function," "capability" |
| **Skill** | Reusable instructions an agent can pull in when needed | "Template," "snippet" |
| **MCP server** | External tool that follows a standard protocol | "context server" |
| **Draining** | Letting current work finish before restarting | "Shutdown," "restart" |

### Code Style

- **Commands:** `snake_case` for file paths, `kebab-case` for CLI args
- **JSON:** 2-space indentation, always valid, never truncated
- **Environment variables:** Always uppercase with `BRIDGE_` prefix
- **File paths:** Use `/home/user/project` style (Unix-forward, relatable)

### Explain Acronyms
First time you use an acronym, spell it out and explain it simply:

**❌ Jargony:**
> Bridge uses SSE for real-time updates.

**✅ Clear:**
> Bridge uses **SSE** (Server-Sent Events) to stream responses to your frontend. It's a simple way for a server to push data to a browser as it becomes available.

### Formatting Conventions

```markdown
## H2: Major sections
### H3: Subsections
**Bold** for UI elements, key terms
`code` for file names, variables, short commands
```
Code blocks``` for multi-line code with language tag

> 💡 **Tip:** Callouts for helpful but non-essential info
> ⚠️ **Warning:** For pitfalls that cause real problems
> 🔍 **Note:** For context, background, or "how it works"
```

---

## 🏗️ Content Patterns by Section Type

### Getting Started Pages
- **Goal:** Confidence through quick wins
- **Tone:** Encouraging, assumes minimal prior knowledge
- **Structure:** Prerequisites → Installation → Verification → Next steps
- **Length:** 3-5 min read per page

### Core Concepts Pages
- **Goal:** Mental model building
- **Tone:** Explanatory, uses analogies where helpful
- **Structure:** Concept → How it works in Bridge → Example → Common patterns
- **Length:** 5-8 min read per page

### API Reference Pages
- **Goal:** Quick lookup with complete accuracy
- **Tone:** Precise, consistent, scannable
- **Structure:**
  ```
  ## Endpoint Name
  METHOD /path/to/endpoint
  
  Brief description (1 sentence)
  
  ### Request
  ```json
  {
    "field": "type"  // Required. Description.
  }
  ```
  
  ### Response
  Status: 200 OK
  ```json
  { ... }
  ```
  
  ### Errors
  | Code | Meaning |
  ```

### Tutorials
- **Goal:** Learn by building
- **Tone:** Guided, conversational, encouraging
- **Structure:** What you'll build → Prerequisites → Step-by-step (with verification at each step) → Full code → What you learned
- **Length:** 15-20 min to complete

### Tool Reference Pages
- **Goal:** Copy-paste ready examples
- **Tone:** Direct, example-heavy
- **Structure:**
  ```
  ## tool_name
  
  One-line description.
  
  ### Parameters
  | Name | Type | Required | Description |
  
  ### Example
  ```json
  { "name": "tool_name", "arguments": { ... } }
  ```
  
  ### Common patterns
  - Pattern 1
  - Pattern 2
  ```

---

## 🌐 Specific Style Decisions

### British vs. American English
**American English** (standard for technical docs):
- "configure" not "configurate"
- "behavior" not "behaviour"
- "color" not "colour"

### Abbreviations
- Spell out on first use: "Model Context Protocol (MCP)"
- Common tech abbreviations OK without expansion: HTTP, API, JSON, URL
- Never use "e.g." or "i.e." — use "for example" and "that is"

### Numbers
- 1-9: spell out ("three retries")
- 10+: numerals ("15 minutes")
- Always numerals with units: "5MB", "2 hours", "version 2"

### Punctuation
- Serial/Oxford comma: Yes ("agents, tools, and skills")
- Periods at end of list items when they're complete sentences
- No period when list items are fragments

---

## 📋 Content Strategy

### Search-First Structure
Every page is optimized for finding information quickly:

1. **H1** contains main keyword
2. **First paragraph** answers the question directly
3. **H2s** form a table of contents on the right
4. **Code blocks** are copy-paste ready
5. **"See also"** at bottom links related content

### Cross-Linking Strategy
- Every concept links to its detailed page on first mention
- "See also" sections at end of every page
- Breadcrumbs or clear "up" navigation

### Example Philosophy
- All examples are **complete and runnable**
- No `...` or `// etc` in code blocks
- Realistic data (e.g., `"agent-id": "support-agent-prod"` not `"foo"`)
- Multiple examples: minimal case → realistic case → advanced case

### Maintenance Strategy
- Code examples tested in CI where possible
- Version tags on all code snippets
- Changelog linked from every page
- "Last updated" footer

---

## ✅ Quality Checklist

Before any page is marked complete:

- [ ] Technical accuracy verified against code
- [ ] All code examples tested and runnable
- [ ] First paragraph answers "what is this?"
- [ ] Screenshots/diagrams where they add clarity
- [ ] Links to related pages
- [ ] No orphan pages (every page linked from nav)
- [ ] Accessible: alt text on images, semantic headings
- [ ] Mobile-readable (no wide tables, code scrolls)

---

## 🎨 Voice in Action: Sample Openings

### Getting Started
> Bridge is an AI runtime: you tell it about your agents, and it runs them. In the next 15 minutes, you'll have your first agent running locally. No cloud accounts, no complex setup — just a terminal and a working conversation.

### Core Concept (Agents)
> What's an agent? It's the complete setup for an AI worker: which AI model to use, what tools it has access to, and what instructions it should follow. 
> 
> You define an agent once, send it to Bridge, and it stays ready to handle conversations. Think of it like hiring someone: you write the job description (the agent definition), and Bridge handles putting them to work.

### API Reference (Push Agents)
> **POST /push/agents**
> 
> Send one or more agent definitions to Bridge. Use this when your app starts up, or whenever your agent configs change.
> 
> **How it works:** If you send an agent Bridge already has with the same version number, Bridge ignores it. If the version is different, Bridge replaces the old one. This means you can safely call this endpoint repeatedly — nothing breaks if you "push" the same thing twice.

### Tutorial
> By the end of this tutorial, you'll have a customer support agent that can check order status, process refunds, and hand off to human agents when needed. It'll connect to your existing systems and respond to customers in real time. Let's build it.

---

## ❓ For Your Review

1. **Voice fit**: Does "senior engineering mentor" match your vision for Bridge?
2. **Technical depth**: Should we go deeper on internals or stay practical?
3. **Personality level**: More formal? More conversational?
4. **Example style**: Prefer realistic business scenarios or simple/generic?
5. **Any terms to avoid**: Company-specific language, competitor references?

---

*Once approved, I will write the complete documentation following these guidelines.*
