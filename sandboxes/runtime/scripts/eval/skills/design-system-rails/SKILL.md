---
name: design-system-rails
description: Rails no-build brand and design-system workflow for creating CSS tokens, component conventions, accessible color/typography choices, and a README brand guide.
category: design
tags: [design-system, branding, css, rails]
---

# Design System for Rails

Use this skill when defining or refining the app's brand system.

## Outputs

- A concise brand guide in `README.md`
- CSS custom properties in `app/assets/stylesheets/application.css`
- Base styles and reusable component conventions
- Frontend handoff notes for layouts, components, and states

## Token Set

Define practical tokens:

- `--font-sans`, `--font-serif`, `--font-mono`
- `--color-bg`, `--color-surface`, `--color-text`, `--color-muted`
- `--color-brand`, `--color-brand-contrast`
- `--color-border`, `--color-focus`
- `--color-success`, `--color-warning`, `--color-danger`
- `--space-*`, `--radius-*`, `--shadow-*`

Use system font stacks unless the product has a clear reason to load external fonts without a build step.

## Quality Rules

- Make the brand specific to the product domain and audience.
- Avoid one-note palettes and generic SaaS gradients.
- Use accent color selectively.
- Keep cards at 8px radius or less unless the brand guide says otherwise.
- Include focus, hover, disabled, empty, error, and success states.
- Preserve accessible contrast.
- Avoid decorative blobs, bokeh, and purely ornamental SVG systems.

## README Brand Guide

Include:

- Brand personality
- Color palette
- Typography
- Layout principles
- Component conventions
- Accessibility notes

Keep it short enough for implementation agents to follow.

## Source Notes

Adapted from public design-system and frontend-design skill patterns, rewritten for Rails ERB and plain CSS.
