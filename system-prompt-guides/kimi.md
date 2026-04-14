# Kimi (Moonshot AI) — System Prompt Guidelines for Agentic Tasks

## Model Lineage

Moonshot AI has released Kimi K2 (MoE, 1T total / 32B active parameters), and Kimi K2.5 (January 2026, adds vision). The older `kimi-latest` was discontinued January 28, 2026. Current flagship: `kimi-k2.5` with 256K context window.

> Source: [Kimi Main Concepts — Kimi API Platform](https://platform.kimi.ai/docs/introduction), [Kimi K2 Instruct — Hugging Face](https://huggingface.co/moonshotai/Kimi-K2-Instruct)

---

## API Compatibility

Kimi is **fully OpenAI-compatible**. Use the OpenAI SDK with `base_url="https://api.moonshot.ai/v1"`. This is the most drop-in-replacement-friendly Chinese LLM API. The domain has recently migrated from `platform.moonshot.ai` to `platform.kimi.ai`.

> Source: [Kimi Main Concepts — Kimi API Platform](https://platform.kimi.ai/docs/introduction)

---

## Default System Prompt

Kimi's recommended default system prompt:
```
You are Kimi, an AI assistant provided by Moonshot AI. You are proficient in Chinese and English conversations. You provide users with safe, helpful, and accurate answers. You refuse to answer questions related to terrorism, racism, or explicit content. Moonshot AI is a proper noun and should not be translated.
```

For the K2 series, a more minimal version is used:
```
You are Kimi, an AI assistant created by Moonshot AI.
```

> Source: [Kimi Chat API Reference](https://platform.kimi.ai/docs/api/chat), [Kimi K2 Instruct — Hugging Face](https://huggingface.co/moonshotai/Kimi-K2-Instruct)

---

## System Prompt Engineering — Key Guidelines

Moonshot's official documentation provides specific prompt engineering guidance:

### 1. Do NOT List Tools in Your System Prompt
This is explicitly stated: "There is no need to specify the tools or their usage in the System Prompt, as this may actually interfere with Kimi K2.5's autonomous decision-making." This is a major difference from OpenAI/Anthropic best practices.

> Source: [Kimi Agent Setup Guide — Kimi API Platform](https://platform.kimi.ai/docs/guide/use-kimi-k2-to-setup-agent)

### 2. Be Extremely Detailed
"The less the model has to guess about your needs, the more likely you are to get satisfactory results."

> Source: [Kimi Prompt Best Practices](https://platform.kimi.ai/docs/guide/prompt-best-practice)

### 3. Use Structural Length Constraints
Request specific paragraph/bullet counts, not word counts. Kimi executes structural constraints more precisely.

> Source: [Kimi Prompt Best Practices](https://platform.kimi.ai/docs/guide/prompt-best-practice)

### 4. Use Delimiters
XML tags, triple quotes, or section headings to distinguish text segments requiring different processing.

> Source: [Kimi Prompt Best Practices](https://platform.kimi.ai/docs/guide/prompt-best-practice)

### 5. Provide Reference Documentation
Direct Kimi to answer exclusively from supplied material with fallback: "I can't find the answer."

> Source: [Kimi Prompt Best Practices](https://platform.kimi.ai/docs/guide/prompt-best-practice)

### 6. Language Matching
All output must match the language of the user's question — including chart titles, axis labels, legends. This is enforced more strictly than Western models.

> Source: [Kimi Prompt Best Practices](https://platform.kimi.ai/docs/guide/prompt-best-practice)

### 7. Role Assignment Works Well
Assign Kimi a defined persona in the system message.

> Source: [Kimi Prompt Best Practices](https://platform.kimi.ai/docs/guide/prompt-best-practice)

---

## Tool/Function Calling Format

Kimi follows the OpenAI tool calling format exactly:

### Tool Definition
```json
{
  "type": "function",
  "function": {
    "name": "function_name",
    "description": "...",
    "parameters": {
      "type": "object",
      "required": ["param"],
      "properties": {
        "param": {"type": "string", "description": "..."}
      }
    }
  }
}
```

> Source: [Kimi Tool Use API — Kimi API Platform](https://platform.kimi.ai/docs/api/tool-use)

### Constraints
- Maximum **128 functions** per request
- Function names must match regex: `^[a-zA-Z_][a-zA-Z0-9-_]{2,63}$`
- "A name that is an easily understandable English word is more likely to be accepted by the model"
- Parallel tool calls supported — model can return multiple `tool_calls` simultaneously
- `tool_choice` only supports `"auto"` or `"none"` (when thinking is enabled). No `"required"` option.

> Source: [Kimi Tool Use API — Kimi API Platform](https://platform.kimi.ai/docs/api/tool-use)

### Response Handling
- `finish_reason: "tool_calls"` indicates tool invocation
- Under `finish_reason="tool_calls"`, the model may include explanatory text in `message.content`
- Tool results must use `role: "tool"` with matching `tool_call_id`

> Source: [Kimi Tool Calling Guide — Kimi API Platform](https://platform.kimi.ai/docs/guide/use-kimi-api-to-complete-tool-calls)

---

## Official Built-in Tools (Unique Feature)

Kimi provides 12 pre-built "official tools" accessible via a Formula system — no setup needed:

| Tool | URI | Purpose |
|------|-----|---------|
| web-search | `moonshot/web-search:latest` | Real-time internet search |
| code_runner | `moonshot/code_runner:latest` | Python execution |
| quickjs | `moonshot/quickjs:latest` | JavaScript sandbox |
| fetch | `moonshot/fetch:latest` | URL content extraction |
| memory | `moonshot/memory:latest` | Persistent storage across sessions |
| rethink | `moonshot/rethink:latest` | Advanced reasoning |
| excel | `moonshot/excel:latest` | CSV/spreadsheet analysis |
| convert | `moonshot/convert:latest` | Unit conversion |
| date | `moonshot/date:latest` | Date/time processing |
| base64 | `moonshot/base64:latest` | Encoding/decoding |
| mew | `moonshot/mew:latest` | Cat blessing generator |
| random-choice | `moonshot/random-choice:latest` | Random selection |

> Source: [Kimi Official Tools Guide](https://platform.kimi.ai/docs/guide/use-official-tools)

### Special `$web_search` Function
This is a built-in function declared as `"type": "builtin_function"` (not `"type": "function"`). It requires no parameter descriptions.

**Critical requirement**: Thinking mode must be disabled when using `$web_search`. Web search results are returned in an encrypted format (`----MOONSHOT ENCRYPTED BEGIN----...----MOONSHOT ENCRYPTED END----`) that the model can consume directly.

> Source: [Kimi Web Search Documentation](https://platform.kimi.ai/docs/guide/use-web-search)

---

## Thinking Mode

Kimi K2.5 supports thinking/reasoning mode:
```json
{"thinking": {"type": "enabled"}}
```
or `"disabled"`. This is a kimi-k2.5-exclusive feature.

> Source: [Kimi Chat API Reference](https://platform.kimi.ai/docs/api/chat)

---

## Unique API Features (Different from OpenAI)

### 1. `prompt_cache_key`
Explicit cache key parameter for session-based optimization, useful for coding agents.

### 2. `partial: true`
Set on the final assistant message for incremental response generation.

### 3. `safety_identifier`
Per-user safety monitoring without exposing PII.

### 4. `cached_tokens`
Explicitly reported in usage statistics.

### 5. File ID References
`ms://<file_id>` for media instead of only URLs.

### 6. Temperature Scaling
Recommended temperature is **0.6** (not 1.0 like most models). For Anthropic-compatible APIs: `real_temperature = request_temperature * 0.6`.

### 7. Rate Limits
At user level, not API key level, shared across all models.

> Source: [Kimi Chat API Reference](https://platform.kimi.ai/docs/api/chat), [Kimi Main Concepts — Kimi API Platform](https://platform.kimi.ai/docs/introduction)

---

## Agent Swarm (Unique Agentic Feature)

Kimi K2.5 introduces **Agent Swarm** — a parallel orchestration framework that decomposes complex tasks into subtasks executed concurrently, reducing latency by up to 4.5x.

Best for: large-scale research, multi-domain analysis, parallel data processing.
Not suitable for: tightly coupled stateful tasks.

> Source: [Kimi K2.5 Prompt Engineering Guide — Prompting Guide](https://www.promptingguide.ai/models/kimi-k2.5)

---

## Agentic Workflow Pattern

The recommended agentic loop:
1. User query with tools defined
2. Model analyzes, autonomously selects tools
3. Execute returned `tool_calls` (support parallel execution)
4. Return results as `role: "tool"` messages
5. Loop until `finish_reason != "tool_calls"`

Key principle: **Let the model decide tool usage autonomously.** Do not over-specify in prompts.

> Source: [Kimi Agent Setup Guide — Kimi API Platform](https://platform.kimi.ai/docs/guide/use-kimi-k2-to-setup-agent)

---

## Key Differentiators (What Makes Kimi Unique)

1. **"Do NOT list tools in system prompt"** — explicitly stated; tool listing interferes with autonomous decision-making
2. **Fully OpenAI-compatible API** — most drop-in replacement of any Chinese LLM
3. **Temperature 0.6** — unique recommended value, with internal scaling
4. **12 built-in official tools** — accessible via Formula URIs, no setup needed
5. **Encrypted web search results** — `$web_search` returns encrypted format consumed directly by model
6. **Agent Swarm** — parallel task decomposition framework (4.5x latency reduction)
7. **Structural over word-count constraints** — paragraphs/bullets, not word counts
8. **Strict language matching** — all output (including chart labels) must match user's language
9. **`prompt_cache_key`** — explicit cache key for session optimization
10. **`tool_choice` limited** — only `"auto"` or `"none"` (no `"required"` option)

---

## Applicable Universal Principles from Internal Guides

These universal principles from the internal framework apply when writing Kimi system prompts:

### Be Extremely Detailed (Aligns with Kimi's Own Guidance)
Kimi's official docs and the universal framework both emphasize: "The less the model has to guess, the better." Provide explicit, action-oriented instructions with expected output formats.

> Source: [Kimi Prompt Best Practices](https://platform.kimi.ai/docs/guide/prompt-best-practice), `prompt-framework.md`

### Examples Over Rules
Since Kimi explicitly says NOT to list tools in system prompts, few-shot examples become the primary way to demonstrate desired agentic behavior (tool selection, output format, reasoning style).

> Source: Universal principle from `prompt-framework.md`

### Tool Safety Tiers
Apply even though Kimi's `tool_choice` only supports `"auto"` or `"none"`. Implement risk gating in your agent loop code:
```python
TOOL_RISK = {
    "search_kb": "low",
    "update_address": "medium",
    "issue_refund": "high",
}
if TOOL_RISK[tool_name] == "high":
    confirmation = await get_human_approval(tool_name, args)
```

> Source: Universal pattern from `agent-guides.md`

### Compaction for Long Sessions
Kimi has a 256K context window, but compaction is still needed for long-running agents. Use structured summarization:
```
"Summarize this conversation. Preserve: decisions made, errors encountered,
constraints discovered, things still to do.
Only exclude: raw file contents, duplicate tool calls."
```

> Source: Universal principle from `anthropic-agent-guide.txt`, `agent-guides.md`

### Human Escalation Triggers
```
"If you fail to understand the intent after 3 attempts:
 say: 'Let me connect you with a specialist.'
 Then call escalate_to_human(reason='intent_unclear')."
```

> Source: Universal pattern from `agent-guides.md`

---

## All Sources

- [Kimi Tool Calling Guide — Kimi API Platform](https://platform.kimi.ai/docs/guide/use-kimi-api-to-complete-tool-calls)
- [Kimi Main Concepts — Kimi API Platform](https://platform.kimi.ai/docs/introduction)
- [Kimi Agent Setup Guide — Kimi API Platform](https://platform.kimi.ai/docs/guide/use-kimi-k2-to-setup-agent)
- [Kimi Tool Use API — Kimi API Platform](https://platform.kimi.ai/docs/api/tool-use)
- [Kimi Official Tools Guide](https://platform.kimi.ai/docs/guide/use-official-tools)
- [Kimi Web Search Documentation](https://platform.kimi.ai/docs/guide/use-web-search)
- [Kimi Chat API Reference](https://platform.kimi.ai/docs/api/chat)
- [Kimi Prompt Best Practices](https://platform.kimi.ai/docs/guide/prompt-best-practice)
- [Kimi K2.5 Prompt Engineering Guide — Prompting Guide](https://www.promptingguide.ai/models/kimi-k2.5)
- [Kimi K2 Instruct — Hugging Face](https://huggingface.co/moonshotai/Kimi-K2-Instruct)
- [Kimi K2.5 System Prompt Extract — GitHub](https://github.com/dnnyngyen/kimi-k2.5-prompts-tools)
