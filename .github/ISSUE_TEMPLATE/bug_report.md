---
name: Bug report
about: Report a reproducible bug
labels: bug
---

<!--
  ============================================================
  BUG REPORT — optimized for both humans and AI agents
  ============================================================

  HARD LIMITS (do not exceed):
    - Title:               max 80 characters, format: "[area] short description"
    - Summary:             max 3 sentences (~300 chars)
    - Steps to reproduce:  max 8 numbered steps
    - Logs / stack traces: max 30 lines inline; link a gist for anything longer
    - Total body:          aim for under 3000 characters

  AGENT GUIDANCE:
    - Fill EVERY section. Use "n/a" or "unknown" rather than deleting sections.
    - Provide a MINIMAL reproduction. Strip unrelated code/config.
    - Reference files with `path/to/file.ts:42`.
    - Quote exact error messages between backticks; do not paraphrase.
    - Distinguish what you OBSERVED from what you INFERRED — do not present guesses as facts.
    - Do NOT speculate about the fix in the report; that goes in a PR.
    - Do NOT paste the entire repo state, full logs, or screenshots of text. Use code blocks for text.
  ============================================================
-->

## Summary

<!-- 1-3 sentences. What is broken, where, and how often. -->

## Job to be done

<!--
  Required. The user-level job this bug is blocking, in JTBD form:
    "When <situation>, I want to <motivation>, so I can <expected outcome>."
  Example: "When I sign up with a Google account, I want my workspace to be created automatically, so I can start inviting teammates without extra setup."
  Keep to 1-2 sentences. Describe the JOB, not the bug.
-->

## Steps to reproduce

1.
2.
3.

## Expected behavior

## Actual behavior

<!-- Include the exact error message in backticks. -->

## Minimal reproduction

<!-- Code snippet, repo link, curl command, or test case. Keep it small. -->

```

```

## Environment

- Version / commit:
- OS:
- Runtime (Go / Node / browser):
- Deployment (local / staging / prod):

## Logs / stack trace

<!-- Max 30 lines. Trim to the relevant frames. Link a gist for the rest. -->

```

```

## Impact & frequency

<!-- Who is affected, how often it happens, any workaround. -->

-

## Additional context

<!-- Related issues, recent changes, anything you tried. Optional. -->
