//! Checkpoint extraction prompts.

/// Default checkpoint extraction prompt.
pub(super) const DEFAULT_CHECKPOINT_PROMPT: &str = "\
You are extracting a structured checkpoint from a conversation that is being \
continued in a fresh context window. The user will not see this output — it will \
be injected into the new context to help the assistant continue seamlessly.

First, think through the entire conversation. Review the user's ORIGINAL high-level \
goal (from the earliest user turns), the actions the assistant took across every \
turn, tool outputs, file modifications, errors encountered, and any unresolved \
questions. Identify every piece of information needed to continue the work. The \
Overall Goal MUST reflect the conversation-wide objective — not just the most \
recent topic.

Then produce the checkpoint with these sections:

## Overall Goal
A single concise sentence describing the user's conversation-wide high-level \
objective (derived from the earliest user turns, not just the latest topic).

## Active Constraints
Explicit constraints, preferences, or rules established by the user or discovered \
during the conversation. Examples: brand voice guidelines, coding style preferences, \
budget limits, target audience, framework choices, regulatory requirements.

## Key Knowledge
Crucial facts and discoveries about the working context. This could be technical \
details (build commands, API endpoints, database schemas), domain knowledge \
(market segments, competitor analysis, audience demographics), or environmental \
facts (team structure, timelines, tool access) — anything the assistant needs to \
know to continue effectively.

## Work Trail
Key artifacts that were produced, modified, or reviewed, and WHY. Track the \
evolution of significant outputs and their rationale. Examples:
- For code: `src/auth.rs`: Refactored from JWT to session tokens for compliance.
- For content: Campaign brief v2: Revised targeting from 18-24 to 25-34 based on analytics.
- For research: Competitive analysis: Added 3 new entrants identified in Q3 reports.

## Key Decisions
Important decisions made during the conversation with brief rationale.

## Task State
The current plan with completion markers:
1. [DONE] Research phase
2. [IN PROGRESS] Draft deliverables  <-- CURRENT FOCUS
3. [TODO] Review and finalize

## Transition Context
A brief paragraph (2-4 sentences) telling the assistant exactly where things \
left off and what to do next. Address the assistant directly.

Keep the checkpoint focused and dense. Aim for under 1200 tokens total — prune \
details that are fully superseded by later decisions.";

/// Gemini-tuned checkpoint prompt. Google's prompt-design docs recommend XML
/// delimiters, critical rules first, explicit length caps per section, and
/// active-verb pruning directives. Without those, Gemini 2.5 Flash accumulates
/// content monotonically across chains (observed going 4k → 7k → 15k bytes over
/// 3 chains with the default prompt). With this prompt it stays roughly flat.
/// Selected automatically by [`default_prompt_for_provider`] when the model
/// string contains "gemini".
pub(super) const GEMINI_CHECKPOINT_PROMPT: &str = "\
<role>
You are a conversation-checkpoint extractor. Your only job: compress a completed \
portion of an LLM conversation into a DENSE structured checkpoint that another \
assistant will read in a fresh context window to continue the work seamlessly. \
The user never sees your output — it is prompt-context injection only.
</role>

<hard_rules>
1. Your entire output MUST be under 900 tokens. Longer output is a failure.
2. Produce EXACTLY the 7 sections below, in order, using the exact markdown \
headings shown. No preamble, no closing remarks. Start your response DIRECTLY \
with the line \"## Overall Goal\".
3. When a <previous_checkpoint> is supplied, you MUST actively PRUNE it. DELETE \
items fully superseded by later decisions. DELETE items marked DONE more than \
one chain ago. MERGE near-duplicate bullets. Do NOT preserve content \"for \
completeness\" — the whole point of a checkpoint is compression, not accumulation.
4. The Overall Goal is the user's CONVERSATION-WIDE objective from the earliest \
user turns. Never narrow it to the most recent topic, even if recent turns focus \
on one sub-area.
5. Every bullet is a concrete specific fact: library names with parameters, \
numeric constants, file paths, decisions with reasons. Forbidden words in bullets: \
\"discussed\", \"covered\", \"explored\", \"looked at\", \"considered\". Write the \
conclusion, not the activity.
6. If a section has no relevant content write \"- (none)\". Do not invent content.
</hard_rules>

<output_template>
## Overall Goal
One sentence, max 30 words — the conversation-wide objective.

## Active Constraints
Bullets. Max 8 items, each ≤20 words. Rules/preferences/limits still binding.

## Key Knowledge
Bullets. Max 12 items, each ≤25 words. Concrete technical facts (libraries + \
versions + parameters, schema details, endpoints, file paths, numeric constants). \
No generalities.

## Work Trail
Bullets. Max 8 items, each in the form \"`<artifact>`: <what changed + why>\". \
Drop items fully superseded.

## Key Decisions
Bullets. Max 8 items, each ≤25 words. Named decisions + one-phrase rationale.

## Task State
Numbered list. Each line tagged [DONE] / [IN PROGRESS] / [TODO]. Mark exactly one \
[IN PROGRESS] with \"<-- CURRENT FOCUS\".

## Transition Context
One paragraph, 2-3 sentences, ≤70 words. Address the assistant directly: where \
things stopped, what to do next.
</output_template>

<pruning_discipline>
When previous_checkpoint(s) are present:
- KEEP: decisions still governing forward work; facts still needed; constraints \
not yet met.
- DROP: tasks marked DONE more than one chain ago; details superseded by later \
decisions; duplicated bullets; narrative framing; prior Transition Context \
paragraphs.
- MERGE: near-duplicate bullets into one tighter bullet.
</pruning_discipline>

Read the entire conversation and any previous checkpoints below, then produce \
your checkpoint. Remember: start with \"## Overall Goal\" on the first line.";

/// Pick the built-in checkpoint prompt that best fits the configured
/// summarizer. Gemini-family models benefit substantially from stricter
/// structure + pruning directives (see [`GEMINI_CHECKPOINT_PROMPT`]); every
/// other provider falls through to the generic default.
pub(super) fn default_prompt_for_provider(
    provider: &bridge_core::provider::ProviderConfig,
) -> &'static str {
    use bridge_core::provider::ProviderType;
    let is_gemini = matches!(provider.provider_type, ProviderType::Google)
        || provider.model.to_ascii_lowercase().contains("gemini");
    if is_gemini {
        GEMINI_CHECKPOINT_PROMPT
    } else {
        DEFAULT_CHECKPOINT_PROMPT
    }
}

/// Verification prompt for the (optional) second phase of checkpoint extraction.
pub(super) const VERIFICATION_PROMPT: &str = "\
Critically evaluate the checkpoint you just generated against the conversation \
history. Did you omit any important details — artifacts produced, user constraints, \
key facts about the working context, or task state? Is the Overall Goal the \
conversation-wide objective, or did you narrow it to the most recent topic? \
If anything important is missing, produce a FINAL improved checkpoint with the \
same section structure. Otherwise, repeat the exact same checkpoint.";
