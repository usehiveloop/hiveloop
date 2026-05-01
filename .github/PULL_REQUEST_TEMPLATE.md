<!--
  ============================================================
  PR TEMPLATE — optimized for both humans and AI agents
  ============================================================

  HARD LIMITS (do not exceed):
    - Title:        max 70 characters, conventional-commit style (e.g. "fix(web): ...")
    - Summary:      max 5 bullets, ~600 characters total
    - Why:          max 3 bullets,  ~300 characters total
    - Test plan:    max 8 checklist items
    - Risk / rollback: max 3 bullets each
    - Total body:   aim for under 4000 characters

  AGENT GUIDANCE:
    - Fill EVERY section. If a section truly does not apply, write "n/a" — do not delete it.
    - Link issues with "Closes #123" / "Refs #123" — use "Closes" only when this PR fully resolves the issue.
    - Do NOT paste large diffs, full logs, or generated output into the body. Link or attach instead.
    - Do NOT include AI-generated boilerplate ("As an AI assistant...", "I have analyzed..."). Just the facts.
    - Keep tense consistent: imperative ("add X", "fix Y"), present-tense for behavior ("returns Z").
    - If you touched migrations, secrets, feature flags, or CI config, call it out under "Risk".
  ============================================================
-->

## Summary

<!-- What changed, in 1-5 bullets. max 40 words. No need to list files. PR diff on github already shows files changed -->

-

## Job done

<!-- Why this change is needed. Link the issue/incident/customer report if there is one. -->

-


-

## Demo

<!--
  Required. Show the change actually working.

  Frontend changes:
    - Attach a short screen recording (≤ 60s, ≤ 10MB) OR before/after screenshots.
    - Include both desktop and mobile if the change is responsive.
    - Drag files directly into this box; GitHub will host them.

  Backend / API / CLI changes:
    - Paste the request and response (curl, httpie, or CLI invocation).
    - Trim to the relevant fields. Redact secrets and PII.
    - For DB / migration changes, show the before/after schema or row counts.

  Pure refactor / internal changes:
    - Write "n/a — internal refactor, behavior unchanged" and link the test that proves it.
-->

```

```

## Requirements met

- [ ] For api changes, I wrote integration tests (database facing tests) to test the real business value behavior
- [ ] For frontend changes, I tested in the browser using agent-browser, and uploaded screenshots to the pr description

## Risk & rollback

<!-- What could break? How do we revert? Migrations, feature flags, deploy ordering, etc. -->

- Risk:
- Rollback:

## Linked issues

<!-- "Closes #123" auto-closes on merge. Use "Refs #123" otherwise. -->

-
