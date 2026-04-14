# GLM (Zhipu AI / Z.AI) — System Prompt Guidelines for Agentic Tasks

## Model Lineage

Zhipu AI (now branded as Z.AI) has released GLM-4, GLM-4.5, GLM-4.6, and GLM-4.7. The latest models are open-source under MIT license. GLM-4.5 has 355B total / 32B active parameters; GLM-4.6 offers a 200K context window.

> Source: [GLM-4.6 Overview — Z.AI Developer Document](https://docs.z.ai/guides/llm/glm-4.6), [GLM-4.5 GitHub Repository](https://github.com/zai-org/GLM-4.5)

---

## API Compatibility

GLM uses an **OpenAI-compatible API** at `https://api.z.ai/api/paas/v4/chat/completions`. You can use the OpenAI Python SDK by setting `base_url="https://api.z.ai/api/paas/v4/"`.

Message roles supported: `system`, `user`, `assistant`, and a **unique `observation` role** (rendered as `<|observation|>` internally) for tool results — though the API also accepts the standard `tool` role.

> Source: [Z.AI Quick Start Guide](https://docs.z.ai/guides/overview/quick-start)

---

## System Prompt Format

System prompts use the standard `{"role": "system", "content": "..."}` format. No special formatting is required.

However, when tools are defined, GLM **automatically injects a tool-description system message** that gets prepended to the conversation. This means if you provide both a system prompt and tools, the tool schema is formatted into an internal system prompt that comes before your custom system prompt.

The internally generated tool system prompt looks like:
```
# Tools

You may call one or more functions to assist with the user query.

You are provided with function signatures within <tools></tools> XML tags:

<tools>
[JSON tool definitions]
</tools>

For each function call, output the function name and arguments within the following XML format:

<tool_call>{function-name}<arg_key>{arg-key-1}</arg_key><arg_value>{arg-value-1}</arg_value>...</tool_call>
```

> Source: [GLM-4.7 Chat Template — Hugging Face](https://huggingface.co/zai-org/GLM-4.7/blob/main/chat_template.jinja), [GLM-4.6 Chat Template — Hugging Face](https://huggingface.co/zai-org/GLM-4.6/blob/main/chat_template.jinja)

---

## Tool/Function Calling — Key Difference from OpenAI

This is the most significant differentiator. While the API accepts OpenAI-style tool definitions, the raw model output uses a proprietary XML-based format, not JSON function calls.

### Tool Call Output Format (Raw Model)
```xml
<tool_call>function_name<arg_key>param1</arg_key><arg_value>value1</arg_value><arg_key>param2</arg_key><arg_value>value2</arg_value></tool_call>
```

### Tool Result Input Format (Observation Role)
```xml
<|observation|>
<tool_response>
{result content}
</tool_response>
```

### Thinking/Reasoning Wrapper
```xml
<think>internal reasoning here</think>
```

If you use the Z.AI hosted API, this XML is parsed server-side and returned in OpenAI-compatible `tool_calls` format. But if you self-host (via vLLM/SGLang), you must configure parsers: `--tool-call-parser glm47` and `--reasoning-parser glm45`.

> Source: [GLM-4.6 Tool Calling & MCP Analysis — Cirra](https://cirra.ai/articles/glm-4-6-tool-calling-mcp-analysis), [Function Calling — Z.AI Developer Document](https://docs.z.ai/guides/capabilities/function-calling), [GLM-4.7 Chat Template — Hugging Face](https://huggingface.co/zai-org/GLM-4.7/blob/main/chat_template.jinja)

---

## Special Tokens and Chat Template

GLM uses unique special tokens not found in Western models:
- `[gMASK]<sop>` — Preamble tokens at the start of every conversation
- `<|system|>`, `<|user|>`, `<|assistant|>`, `<|observation|>` — Role markers
- `<think>` / `</think>` — Reasoning content delimiters
- `<tool_call>` / `</tool_call>`, `<arg_key>` / `<arg_value>` — Tool invocation XML tags
- `/nothink` — Appended to user messages in GLM-4.6 to disable thinking mode

> Source: [GLM-4.7 Chat Template — Hugging Face](https://huggingface.co/zai-org/GLM-4.7/blob/main/chat_template.jinja), [GLM-4.6 Chat Template — Hugging Face](https://huggingface.co/zai-org/GLM-4.6/blob/main/chat_template.jinja)

---

## Thinking Modes (Unique Feature)

GLM offers three thinking configurations critical for agentic use:

### 1. Interleaved Thinking (GLM-4.7)
The model thinks before every response AND before every tool call. This is fundamentally different from OpenAI/Anthropic where reasoning happens once per turn.

### 2. Preserved Thinking (GLM-4.7)
Retains `<think>` blocks across multi-turn conversations, reusing prior reasoning instead of re-deriving from scratch. Configured via:
```json
{"chat_template_kwargs": {"enable_thinking": true, "clear_thinking": false}}
```

### 3. Turn-Level Control
Per-turn enable/disable of thinking within a session — disable for lightweight requests, enable for complex tool-use chains.

API parameter: `"thinking": {"type": "enabled"}` or `"disabled"`

> Source: [GLM-4.6 Overview — Z.AI Developer Document](https://docs.z.ai/guides/llm/glm-4.6), [GLM-4.7 Chat Template — Hugging Face](https://huggingface.co/zai-org/GLM-4.7/blob/main/chat_template.jinja)

---

## Built-in Tools (GLM-4 AllTools)

The older GLM-4 AllTools model includes pre-integrated tools:
- `python` — Code interpreter with sandbox
- `browser.search` — Web search with `query` and `num` parameters
- `browser.open` — Navigate to URLs
- `browser.find` — Pattern matching in loaded pages
- `drawing_tool` — Image generation

These are declared with `"type": "code_interpreter"`, `"type": "web_browser"`, or `"type": "drawing_tool"` rather than `"type": "function"`.

> Source: [Function Calling — Z.AI Developer Document](https://docs.z.ai/guides/capabilities/function-calling)

---

## Agentic Behavior Characteristics

- GLM-4.6+ demonstrates **autonomous tool orchestration** — the model independently determines when tools are needed during generation, rather than requiring explicit function declarations upfront
- Specifically designed to **refuse unknown tools and minimize invented arguments** — targeting zero hallucination in tool calls
- Tight JSON schema adherence — fails explicitly rather than silently correcting malformed output
- Recommended temperature: **1.0** for consistent tool-calling behavior
- `tool_choice` currently supports only `"auto"` mode

> Source: [GLM-4.6 Tool Calling & MCP Analysis — Cirra](https://cirra.ai/articles/glm-4-6-tool-calling-mcp-analysis), [GLM-4.6 Overview — Z.AI Developer Document](https://docs.z.ai/guides/llm/glm-4.6)

---

## System Prompt Best Practices

### Keep System Prompts Minimal
Because GLM auto-injects tool schemas into the system prompt layer, your custom system prompt should focus on:
- Agent identity and role
- Behavioral constraints
- Output format preferences
- Domain-specific context

Do not duplicate tool descriptions in your system prompt — they are already injected.

> Source: Inferred from [GLM-4.7 Chat Template — Hugging Face](https://huggingface.co/zai-org/GLM-4.7/blob/main/chat_template.jinja)

### Temperature
Recommended: **1.0** for consistent tool-calling behavior. This aligns with Gemini and MiniMax recommendations but contrasts with Kimi (0.6).

> Source: [GLM-4.6 Overview — Z.AI Developer Document](https://docs.z.ai/guides/llm/glm-4.6)

### No Explicit Prompt Engineering Guide
Unlike Anthropic and OpenAI, Zhipu AI does not publish a comprehensive prompt engineering guide. The model is designed to be OpenAI-compatible, so OpenAI prompt engineering principles generally apply with the following exceptions:
- The XML-based tool call format (handled by API layer)
- The observation role for tool results
- Interleaved and preserved thinking
- Auto-injected tool system prompts

> Source: General observation from documentation survey

---

## Performance Context

- CC-Bench (multi-turn coding): 48.6% win-rate vs Claude Sonnet 4
- BrowseComp (web browsing): 90.6% success rate (GLM-4.5)
- 15% fewer tokens than predecessors for equivalent tasks

> Source: [GLM-4.5 GitHub Repository](https://github.com/zai-org/GLM-4.5), [GLM-4.6 Overview — Z.AI Developer Document](https://docs.z.ai/guides/llm/glm-4.6)

---

## Key Differentiators (What Makes GLM Unique)

1. **Proprietary XML tool call format** — `<tool_call>`, `<arg_key>`, `<arg_value>` internally (API abstracts to OpenAI format)
2. **Observation role** — unique `<|observation|>` role for tool results alongside standard `tool` role
3. **Interleaved thinking** — thinks before every response AND every tool call (not just once per turn)
4. **Preserved thinking** — retains reasoning across multi-turn conversations, reusing prior analysis
5. **Per-turn thinking control** — enable/disable thinking on individual turns within a session
6. **Auto-injected tool system prompt** — tool schemas automatically formatted and prepended
7. **Zero-hallucination tool call design** — refuses unknown tools, minimizes invented arguments
8. **`[gMASK]<sop>` preamble** — unique special tokens at conversation start

---

## Applicable Universal Principles from Internal Guides

Since GLM lacks a comprehensive prompt engineering guide, these universal principles from the internal framework (`prompt-framework.md`, `agent-guides.md`) apply when writing GLM system prompts:

### Structure with Clear Sections
Use XML tags or Markdown headers consistently. GLM's internal tool injection uses XML, so XML is the natural fit:
```xml
<role>You are a senior code reviewer.</role>
<rules>
- Flag security vulnerabilities as blocking.
- Flag style issues as non-blocking.
</rules>
<output_format>
Return a JSON array of findings: {file, line, severity, message}
</output_format>
```

> Source: Universal principle from `prompt-framework.md`

### Start Minimal, Iterate on Failures
Begin with minimal instructions and only add guidance when you observe specific failures. Every line should trace back to a real failure in testing.

> Source: Universal principle from `anthropic-agent-guide.txt`, `prompt-framework.md`

### Examples Over Rules
3 well-chosen examples teach behavior more reliably than 15 rules. Since GLM has limited prompt engineering documentation, examples are especially important for steering behavior.

> Source: Universal principle from `prompt-framework.md`

### Token-Efficient Tool Responses
GLM's tool injection adds token overhead. Keep tool responses lean — return only the fields the agent needs, not entire API responses.

> Source: Universal principle from `anthropic-agent-guide.txt`

### Tool Safety Tiers
Apply risk-based tool gating even though GLM only supports `tool_choice: "auto"`:
```
Low risk (proceed freely): search, read, list
Medium risk (proceed + log): update, create
High risk (always confirm): delete, payment, cancel
```

> Source: Universal pattern from `prompt-framework.md`, `agent-guides.md`

---

## All Sources

- [GLM-4.6 Overview — Z.AI Developer Document](https://docs.z.ai/guides/llm/glm-4.6)
- [GLM-4.6 Tool Calling & MCP Analysis — Cirra](https://cirra.ai/articles/glm-4-6-tool-calling-mcp-analysis)
- [Function Calling — Z.AI Developer Document](https://docs.z.ai/guides/capabilities/function-calling)
- [Z.AI Quick Start Guide](https://docs.z.ai/guides/overview/quick-start)
- [GLM-4.7 Chat Template — Hugging Face](https://huggingface.co/zai-org/GLM-4.7/blob/main/chat_template.jinja)
- [GLM-4.6 Chat Template — Hugging Face](https://huggingface.co/zai-org/GLM-4.6/blob/main/chat_template.jinja)
- [GLM-4.5 GitHub Repository](https://github.com/zai-org/GLM-4.5)
- [GLM-4.7 AIML API Docs](https://docs.aimlapi.com/api-references/text-models-llm/zhipu/glm-4.7)
