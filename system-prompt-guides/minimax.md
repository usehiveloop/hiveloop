# MiniMax — System Prompt Guidelines for Agentic Tasks

## Model Lineup and Evolution

MiniMax has gone through several model generations:

- **abab series** (abab-5.5, abab-6, abab-6.5, abab-6.5s): Legacy models. Essentially deprecated in favor of the M-series. Very little prompt engineering documentation exists for them.
- **MiniMax-Text-01 / MiniMax-01**: Hybrid attention architecture (Lightning Attention + Softmax Attention + MoE). 456B total parameters, 45.9B active per token. Up to 1M token context.
- **MiniMax-M1** (M1-40k, M1-80k): First open-weight hybrid-attention reasoning model. 456B total / 45.9B active. Focused on mathematical and coding reasoning.
- **MiniMax-M2**: 230B total / 10B active (MoE). The first model explicitly "born for agents and code." Introduced interleaved thinking.
- **MiniMax-M2.1**: More concise responses, improved multilingual programming, reduced token consumption.
- **MiniMax-M2.5**: SOTA on SWE-Bench Verified (80.2%). Heavily RL-trained across hundreds of thousands of real-world environments. 204,800 token context.
- **MiniMax-M2.7**: Latest. Self-evolving model. Agent Teams support, 40+ complex skills, 97% skill adherence. 229B parameters.

Current recommended models for agentic use: **MiniMax-M2.5** and **MiniMax-M2.7**.

> Source: [MiniMax M2.7 News Announcement](https://www.minimax.io/news/minimax-m27-en), [MiniMax M2.5 News Announcement](https://www.minimax.io/news/minimax-m25), [MiniMax M2 & Agent Announcement](https://www.minimax.io/news/minimax-m2), [MiniMax M2.7 — Hugging Face](https://huggingface.co/MiniMaxAI/MiniMax-M2.7)

---

## API Compatibility — Dual SDK Support

MiniMax offers **both** OpenAI-compatible and Anthropic-compatible API endpoints:

### OpenAI-Compatible Endpoint
- Base URL: `https://api.minimax.io/v1`
- Standard OpenAI SDK works with `base_url` override
- Supports `tools` parameter (NOT deprecated `function_call`)
- Special extension: `extra_body={"reasoning_split": True}` to separate thinking into `reasoning_details` field
- Some parameters ignored: `presence_penalty`, `frequency_penalty`, `logit_bias`

### Anthropic-Compatible Endpoint
- Base URL: `https://api.minimax.io/anthropic`
- Standard Anthropic SDK works
- Supports `system`, `messages`, `tools`, `tool_choice`, `thinking`
- Some parameters ignored: `top_k`, `stop_sequences`, `service_tier`, `mcp_servers`, `context_management`, `container`
- No image/document support

### China Endpoint
`https://api.minimaxi.com/` (same formats)

> Source: [MiniMax OpenAI-Compatible API](https://platform.minimax.io/docs/api-reference/text-openai-api), [MiniMax Anthropic-Compatible API](https://platform.minimax.io/docs/api-reference/text-anthropic-api)

---

## System Prompt Format and Default Behavior

### Default System Prompt
If no system prompt is provided, the model uses:
```
You are a helpful assistant. Your name is MiniMax-M2.7 and is built by MiniMax.
```

> Source: [MiniMax Text Generation Docs](https://platform.minimax.io/docs/guides/text-generation)

### Official Task-Specific Recommendations

| Task Type | Recommended System Prompt |
|-----------|--------------------------|
| General (Q&A, translation, summarization, creative writing) | `You are a helpful assistant.` |
| Web development / Code generation | `You are a web development engineer, writing web pages according to the instructions below.` |
| Code editing / Artifacts | `You are a powerful code editing assistant capable of writing code and creating artifacts in conversations with users, or modifying and updating existing artifacts as requested by users.` |
| Mathematical reasoning | `Please reason step by step, and put your final answer within \boxed{}.` |
| Long tasks | Include: `This is a very lengthy task...keep total input and output tokens within 200k tokens. Make full use of the context window length to complete the task thoroughly.` |

> Source: [MiniMax Text Generation Docs](https://platform.minimax.io/docs/guides/text-generation)

### Chat Template Special Tokens (Internal Format)
The raw chat template uses these internal markers:
- `]~!b[]~b]system` — system message start
- `]~b]user` — user message
- `]~b]ai` — assistant message
- `]~b]tool` — tool result message
- `[e~[` — message end marker
- `<think>...</think>` — reasoning/thinking blocks
- `<minimax:tool_call>...</minimax:tool_call>` — tool call wrapper

The system message also supports `current_date` and `current_location` fields that get appended automatically.

When using the API (OpenAI or Anthropic SDK), you do NOT need to handle these special tokens yourself.

> Source: [MiniMax M2.7 Chat Template — Hugging Face](https://huggingface.co/MiniMaxAI/MiniMax-M2.7/blob/main/chat_template.jinja)

---

## Tool Use / Function Calling Format

### How Tools Are Injected into the System Prompt
When tools are provided, the chat template automatically appends this to the system message:

```
# Tools
You may call one or more tools to assist with the user query.
Here are the tools available in JSONSchema format:

<tools>
<tool>{"name": "function_name", "description": "...", "parameters": {...}}</tool>
<tool>{"name": "another_function", "description": "...", "parameters": {...}}</tool>
</tools>

When making tool calls, use XML format to invoke tools and pass parameters:

<minimax:tool_call>
<invoke name="tool-name-1">
<parameter name="param-key-1">param-value-1</parameter>
<parameter name="param-key-2">param-value-2</parameter>
...
</invoke>
</minimax:tool_call>
```

> Source: [MiniMax M2.7 Chat Template — Hugging Face](https://huggingface.co/MiniMaxAI/MiniMax-M2.7/blob/main/chat_template.jinja)

### Tool Definition Format (API Level)

**OpenAI SDK format:**
```python
tools = [{
    "type": "function",
    "function": {
        "name": "get_weather",
        "description": "Get weather of a location",
        "parameters": {
            "type": "object",
            "properties": {
                "location": {"type": "string", "description": "City and state"}
            },
            "required": ["location"]
        }
    }
}]
```

**Anthropic SDK format:**
```python
tools = [{
    "name": "get_weather",
    "description": "Get weather of a location",
    "input_schema": {
        "type": "object",
        "properties": {
            "location": {"type": "string", "description": "City and state"}
        },
        "required": ["location"]
    }
}]
```

> Source: [MiniMax Tool Use & Interleaved Thinking — Official Docs](https://platform.minimax.io/docs/guides/text-m2-function-call)

### Model's Tool Call Output (Raw Format)
The model outputs tool calls in XML:
```xml
<minimax:tool_call>
<invoke name="search_web">
<parameter name="query_list">["OpenAI latest release"]</parameter>
</invoke>
</minimax:tool_call>
```

> Source: [MiniMax M2.7 Tool Calling Guide — GitHub](https://github.com/MiniMax-AI/MiniMax-M2.7/blob/main/docs/tool_calling_guide.md)

### Tool Results Are Returned As

**OpenAI format:**
```python
{"role": "tool", "tool_call_id": "...", "content": "result_string"}
```

**Anthropic format:**
```python
{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "...", "content": "result_string"}]}
```

In the raw template, tool results are wrapped in `<response>...</response>` tags.

> Source: [MiniMax Tool Use & Interleaved Thinking — Official Docs](https://platform.minimax.io/docs/guides/text-m2-function-call)

### Legacy Format (VL-01 / Text-01)
Older models (MiniMax-VL-01, MiniMax-Text-01) used a different format:
- Tools injected with `function_setting=functions` marker
- Output as TypeScript code blocks: `` ```typescript\nfunctions.get_weather({"location": "Shanghai"})\n``` ``
- This is NOT the format for M2-series models

> Source: [MiniMax VL-01 Function Call Guide — Hugging Face](https://huggingface.co/MiniMaxAI/MiniMax-VL-01/blob/main/MiniMax-VL-01_Function_Call_Guide.md)

---

## Interleaved Thinking — Critical for Agentic Performance

This is the single most important feature for agentic use cases with MiniMax models.

### What It Is
The model reasons *between* tool calls using `<think>...</think>` blocks, carrying forward plans, hypotheses, and constraints across steps. This is a plan -> act -> reflect loop.

### Performance Impact of Preserving Thinking State

| Benchmark | Without Thinking Preservation | With Thinking Preservation | Improvement |
|-----------|-------------------------------|---------------------------|-------------|
| SWE-Bench Verified | baseline | +3.3% | significant |
| BrowseComp | baseline | +40.1% | massive |
| Tau-squared | baseline | +35.9% | massive |
| GAIA | baseline | +11.5% | significant |

> Source: [MiniMax Interleaved Thinking Importance](https://www.minimax.io/news/why-is-interleaved-thinking-important-for-m2)

### Critical Implementation Requirement
**You MUST preserve the model's complete response (including thinking/reasoning content) in the conversation history when making multi-turn tool calls.** Stripping `<think>` blocks or `reasoning_details` causes severe performance degradation.

**OpenAI SDK with reasoning_split=True:**
```python
response = client.chat.completions.create(
    model="MiniMax-M2.7",
    messages=messages,
    tools=tools,
    extra_body={"reasoning_split": True}
)
# Preserve BOTH the message AND reasoning_details in history
```

**Anthropic SDK:** Append the complete `response.content` list (including thinking blocks) to message history.

> Source: [MiniMax Tool Use & Interleaved Thinking — Official Docs](https://platform.minimax.io/docs/guides/text-m2-function-call), [MiniMax M2.7 Tool Calling Guide — GitHub](https://github.com/MiniMax-AI/MiniMax-M2.7/blob/main/docs/tool_calling_guide.md)

### Configuration Options
1. `reasoning_split=True` (OpenAI SDK): Separates thinking into `reasoning_details` field. Recommended for easier parsing.
2. `reasoning_split=False` (OpenAI SDK): Thinking embedded as `<think>` tags in `content` field. Do NOT modify the content.
3. Anthropic SDK: Natively supports interleaved thinking blocks.

> Source: [MiniMax Tool Use & Interleaved Thinking — Official Docs](https://platform.minimax.io/docs/guides/text-m2-function-call)

---

## Recommended Inference Parameters

Consistently recommended across all MiniMax documentation:

```
temperature = 1.0
top_p = 0.95
top_k = 40
```

MiniMax explicitly states that `temperature=1.0` "allows the model to explore a wider range of linguistic possibilities, preventing outputs that are too rigid or repetitive." This is notably higher than many other providers' defaults.

> Source: [MiniMax M2.7 — Hugging Face](https://huggingface.co/MiniMaxAI/MiniMax-M2.7), [MiniMax M2.5 — Hugging Face](https://huggingface.co/MiniMaxAI/MiniMax-M2.5)

---

## Prompt Engineering Best Practices for MiniMax (Agentic)

### From Official M2.7 Usage Tips

#### 1. Instruction Clarity
Provide explicit, detailed instructions with expected output formats. Instead of "Create a visualization website," say "Create an enterprise-grade data visualization website with rich analytical features and interactive functions beyond basic displays."

#### 2. Explain Intent
Articulate the "why" behind requests. Instead of "Do not use document symbols," explain "Your response will be read aloud by text-to-speech, so use plain text without formatting." M2.7 generalizes better when it understands context and reasoning.

#### 3. Template-Based Guidance
Show good AND bad examples. Include a well-written sample, explicitly highlight mistakes to avoid, then present the actual task.

#### 4. Long-Task Workflow Strategy
- Focus on limited goals per interaction rather than parallel processing
- Be aware M2.7 may terminate tasks early when approaching context capacity
- Use multi-window approach: framework setup in one window, iteration in another
- Create test files to track progress
- Use initialization scripts to reduce repetitive operations

> Source: [MiniMax M2.7 Usage Tips](https://platform.minimax.io/docs/token-plan/best-practices)

### From Third-Party Analysis

#### 5. Structured Prompts with Five Elements
- STRUCTURE: Define components/sections needed
- CONSTRAINTS: Specify tech stack and patterns
- NAMING: Provide file/component names
- CONTEXT: Reference existing project patterns
- DON'T: List explicit exclusions (MiniMax tends toward "creative drift" without constraints)

#### 6. For Code
Always ask for a minimal diff or focused patch, not a full rewrite. Add specific nudges like "Show the exact await chain and event loop ownership."

#### 7. For Agent Tasks
Leverage M2.5/M2.7's planning tendency by requesting architectural specs upfront. The model naturally does "spec-writing" before coding.

> Source: [MiniMax Prompting Strategies — BSWEN](https://docs.bswen.com/blog/2026-03-31-minimax-prompting-strategies/)

---

## Prompt Caching Implications for System Prompts

MiniMax uses **automatic prefix caching** with a specific construction order:
1. Tool list (first)
2. System prompts
3. User messages

### Key Rules
- Minimum 512 input tokens for caching to activate (lowest of any provider)
- Place static/repeated content at the beginning
- Put dynamic user information at the end
- Changes to any earlier module invalidate the cache for everything after it
- Cache hit tokens are billed at ~90% discount

This means: keep your system prompt and tool definitions stable across requests to maximize cache hits.

> Source: [MiniMax Prompt Caching](https://platform.minimax.io/docs/api-reference/text-prompt-caching)

---

## MiniMax's Agentic Architecture (Mini-Agent Reference)

MiniMax's official Mini-Agent demo project reveals their recommended agentic architecture:

- **Execution loop**: Up to 100 steps (configurable `max_steps`)
- **Built-in tools**: File system operations, shell command execution, Session Note Tool (persistent memory)
- **Context management**: Automatic conversation summarization at configurable token limits, enabling theoretically unlimited task lengths
- **MCP integration**: Native Model Context Protocol support for external tools (knowledge graphs, web search, git)
- **Skills system**: 15+ professional skills for document processing, design, testing, development
- **Interleaved thinking**: Fully enabled for complex reasoning between tool calls
- **API**: Uses Anthropic-compatible endpoint

> Source: [MiniMax Mini-Agent — GitHub](https://github.com/MiniMax-AI/Mini-Agent), [Mini-Agent Docs — Official](https://platform.minimax.io/docs/token-plan/mini-agent)

---

## Known Quirks and Issues

### 1. ChatGPT Distillation Artifacts
When running M2 GGUF locally, users have reported the model's `<think>` blocks referencing ChatGPT instructions ("You are ChatGPT, a large language model trained by OpenAI") even when custom system prompts are provided. This is suspected to be a training data artifact. This does NOT affect the hosted API.

> Source: [MiniMax M2-GGUF System Prompt Discussion — Hugging Face](https://huggingface.co/unsloth/MiniMax-M2-GGUF/discussions/2)

### 2. Temperature Requirement
Temperature must be in range (0.0, 1.0] — values outside this range cause errors. The recommended value of 1.0 is unusually high compared to other providers.

> Source: [MiniMax M2.7 — Hugging Face](https://huggingface.co/MiniMaxAI/MiniMax-M2.7)

### 3. No Image/Document Support
Even on the Anthropic-compatible endpoint, image and document content blocks are not supported.

> Source: [MiniMax Anthropic-Compatible API](https://platform.minimax.io/docs/api-reference/text-anthropic-api)

### 4. Early Termination on Long Tasks
M2.7 may stop tasks early when approaching context capacity thresholds. The official recommendation is to include explicit instructions about utilizing the full context window.

> Source: [MiniMax M2.7 Usage Tips](https://platform.minimax.io/docs/token-plan/best-practices)

### 5. Deprecated Parameters
`function_call` (old OpenAI format) is not supported; must use `tools`. Also `n` parameter is limited to 1.

> Source: [MiniMax OpenAI-Compatible API](https://platform.minimax.io/docs/api-reference/text-openai-api)

---

## Key Differentiators (What Makes MiniMax Unique)

1. **Dual API compatibility** — both OpenAI and Anthropic SDKs work, just change base URL
2. **Interleaved thinking is critical** — preserving `<think>` blocks gave +40% on BrowseComp
3. **XML tool call format** — `<minimax:tool_call>` internally (API abstracts it)
4. **512-token minimum for caching** — lowest threshold of any provider
5. **Temperature 1.0 mandatory** — values outside (0, 1.0] cause errors
6. **Explain the "why"** — model generalizes better with motivation (shares this with Anthropic)
7. **Early termination on long tasks** — must explicitly instruct about context window usage
8. **Mini-Agent reference architecture** — official demo with 100-step loops, MCP, and automatic summarization
9. **Self-evolving model** — M2.7 participates in improving itself
10. **97% skill adherence** — M2.7 follows complex multi-step skill instructions reliably

---

## Applicable Universal Principles from Internal Guides

These universal principles from the internal framework apply when writing MiniMax system prompts:

### Explain the "Why" (Matches MiniMax's Own Guidance)
Both MiniMax's official docs and Anthropic's framework agree: explaining intent behind instructions helps the model generalize. MiniMax M2.7 specifically benefits from motivation context.

> Source: [MiniMax M2.7 Usage Tips](https://platform.minimax.io/docs/token-plan/best-practices), `prompt-framework.md`

### Structure with Clear Sections
MiniMax's internal tool injection uses XML (`<minimax:tool_call>`), so XML-structured system prompts are a natural fit:
```xml
<role>You are a deployment assistant.</role>
<instructions>
When the user wants to ship code, verify tests pass before deploying.
If tests fail, diagnose the failure and suggest fixes before retrying.
</instructions>
<constraints>
Never deploy with failing tests.
</constraints>
```

> Source: Universal principle from `prompt-framework.md`

### Compaction Is Critical for MiniMax
MiniMax M2.7 may terminate tasks early when approaching context capacity. Combine explicit context window instructions with compaction:
```
"This is a lengthy task. Keep total input and output tokens within 200k.
Make full use of the context window length to complete the task thoroughly."
```

Additionally implement compaction in your agent loop:
```python
for msg in message_history[:-10]:
    if msg.role == "tool_result":
        msg.content = f"[Result from {msg.tool_name} — already processed]"
```

> Source: [MiniMax M2.7 Usage Tips](https://platform.minimax.io/docs/token-plan/best-practices), `anthropic-agent-guide.txt`

### Token-Efficient Tool Responses
MiniMax's 512-token minimum cache threshold makes this especially important — lean tool responses maximize cache hit rates:
```python
def search_customer(email):
    raw = api.get_customer(email)
    return {"name": raw.name, "plan": raw.plan, "status": raw.status}
    # NOT: return raw (40 fields)
```

> Source: Universal principle from `anthropic-agent-guide.txt`, cache behavior from [MiniMax Prompt Caching](https://platform.minimax.io/docs/api-reference/text-prompt-caching)

### Multi-Window Workflow Strategy
MiniMax's own docs recommend this. The internal framework adds structure:
- Window 1: Framework setup — write tests, create init scripts, establish patterns
- Window 2+: Iterate on implementation, running tests after each feature
- Use structured note-taking to persist state between windows

> Source: [MiniMax M2.7 Usage Tips](https://platform.minimax.io/docs/token-plan/best-practices), `anthropic-agent-guide.txt`

---

## All Sources

- [MiniMax Tool Use & Interleaved Thinking — Official Docs](https://platform.minimax.io/docs/guides/text-m2-function-call)
- [MiniMax M2.7 Usage Tips](https://platform.minimax.io/docs/token-plan/best-practices)
- [MiniMax OpenAI-Compatible API](https://platform.minimax.io/docs/api-reference/text-openai-api)
- [MiniMax Anthropic-Compatible API](https://platform.minimax.io/docs/api-reference/text-anthropic-api)
- [MiniMax Prompt Caching](https://platform.minimax.io/docs/api-reference/text-prompt-caching)
- [MiniMax M2.7 Tool Calling Guide — GitHub](https://github.com/MiniMax-AI/MiniMax-M2.7/blob/main/docs/tool_calling_guide.md)
- [MiniMax M2.5 Tool Calling Guide — GitHub](https://github.com/MiniMax-AI/MiniMax-M2.5/blob/main/docs/tool_calling_guide.md)
- [MiniMax M2.7 Chat Template — Hugging Face](https://huggingface.co/MiniMaxAI/MiniMax-M2.7/blob/main/chat_template.jinja)
- [MiniMax M2.7 — Hugging Face](https://huggingface.co/MiniMaxAI/MiniMax-M2.7)
- [MiniMax M2.5 — Hugging Face](https://huggingface.co/MiniMaxAI/MiniMax-M2.5)
- [MiniMax Mini-Agent — GitHub](https://github.com/MiniMax-AI/Mini-Agent)
- [Mini-Agent Docs — Official](https://platform.minimax.io/docs/token-plan/mini-agent)
- [MiniMax M2.7 News Announcement](https://www.minimax.io/news/minimax-m27-en)
- [MiniMax M2.5 News Announcement](https://www.minimax.io/news/minimax-m25)
- [MiniMax M2 & Agent Announcement](https://www.minimax.io/news/minimax-m2)
- [Forge: Scalable Agent RL Framework](https://www.minimax.io/news/forge-scalable-agent-rl-framework-and-algorithm)
- [MiniMax Interleaved Thinking Importance](https://www.minimax.io/news/why-is-interleaved-thinking-important-for-m2)
- [MiniMax M2.7 on NVIDIA — Technical Blog](https://developer.nvidia.com/blog/minimax-m2-7-advances-scalable-agentic-workflows-on-nvidia-platforms-for-complex-ai-applications/)
- [MiniMax M1 — GitHub](https://github.com/MiniMax-AI/MiniMax-M1)
- [MiniMax VL-01 Function Call Guide — Hugging Face](https://huggingface.co/MiniMaxAI/MiniMax-VL-01/blob/main/MiniMax-VL-01_Function_Call_Guide.md)
- [MiniMax M2-GGUF System Prompt Discussion — Hugging Face](https://huggingface.co/unsloth/MiniMax-M2-GGUF/discussions/2)
- [MiniMax Text Generation Docs](https://platform.minimax.io/docs/guides/text-generation)
- [MiniMax Prompting Strategies — BSWEN](https://docs.bswen.com/blog/2026-03-31-minimax-prompting-strategies/)
- [MiniMax abab6.5 Series Announcement](https://www.minimax.io/news/abab65-series)
- [MiniMax LiteLLM Integration](https://docs.litellm.ai/docs/providers/minimax)
