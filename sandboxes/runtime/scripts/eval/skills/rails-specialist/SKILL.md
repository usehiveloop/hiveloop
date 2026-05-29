---
name: rails-specialist
description: Rails 8.1 no-build specialist for app design and execution. Use this skill before any implementation work. It is the authoritative Rails guide in this eval. For a deep-dive on a specific topic (models, controllers, views, Hotwire/Turbo/Stimulus, testing, or no-build constraints), call skill_view with the relevant sub-skill name (rails-ai-models, rails-ai-controllers, rails-ai-views, rails-ai-hotwire, rails-ai-testing, rails-no-build).
category: rails
tags: [rails, no-build, hotwire, sqlite, minitest, models, controllers, views, testing, turbo, stimulus]
pinned: true
---

# Rails 8.1 Specialist

Use this skill when implementing Rails 8.1 no-build application code inside `/workspace/app`.

## Stack Contract (NON-NEGOTIABLE)

- Rails 8.1.x defaults, SQLite for development and test
- ERB, layouts, partials, helpers, forms, route helpers
- Propshaft-managed assets, Import Maps for JavaScript
- Turbo (Drive, Frames, Streams, Morph) and Stimulus for progressive UI
- Plain CSS in `app/assets/stylesheets`, plain JS in `app/javascript`
- Minitest with Rails default test helpers

NEVER add: React, Vue, Svelte, Vite, Webpack, esbuild, Bun, Tailwind, npm packages, PostCSS, Sass, CSS build pipelines, or client-side SPA routing. If the user asks for a library needing a build step, recommend the smallest Rails-native alternative.

## Architecture Rules

1. **RESTful routes only** — resourceful routes, no custom actions. Nested resources use shallow nesting and module namespacing. No deeper than 1 nesting level.
2. **Skinny controllers** (<50 lines total, actions <10 lines) — move logic to models or service objects. Only REST actions: index, show, new, create, edit, update, destroy.
3. **Fat models, not fat controllers** — validations, associations, scopes, callbacks, enums, custom validators in models. Query objects in `app/queries/`, form objects in `app/forms/`.
4. **Strong parameters always** — `params.expect(model: [...])` or `params.require(:model).permit(...)` for all user input.
5. **Database constraints** — NOT NULL, foreign keys, unique indexes alongside model validations.
6. **No N+1 queries** — use `.includes()` in scopes and controller queries.
7. **Use syntax:`params.expect` for Rails 8.1** — preferred over `params.require`.

## Models (call `skill_view("rails-ai-models")` for full reference)

- Associations at top: `belongs_to`, `has_one`, `has_many`, `has_many :through`
- Validations: `presence`, `uniqueness`, `length`, `numericality`, `format`, `inclusion`
- Scopes are chainable, lazy-evaluated, composable. Use for filters, not class methods.
- Enums: `enum :status, { pending: 0, active: 1, archived: 2 }, prefix: true, scopes: true`
- Callbacks sparingly: `before_validation` for data normalization, `after_commit` for side effects. Complex logic → service objects.
- Transactions: `ActiveRecord::Base.transaction` for multi-model writes.

## Controllers (call `skill_view("rails-ai-controllers")` for full reference)

- Only 7 REST actions. For domain operations (archive, publish, approve), create child controllers with their own `create` action.
- Before actions for setup (`set_feedback`), never for business logic.
- Proper HTTP status codes: 200, 201, 302, 422, 404.
- Render patterns: `redirect_to` with notice, `render :new` with 422 on failure.
- API: `render json:`, appropriate status codes, rescue `RecordNotFound`.

## Views (call `skill_view("rails-ai-views")` for full reference)

- ERB templates, layouts, partials. Rails form builders and route helpers.
- Semantic HTML: landmarks, headings, labels, button types, form errors.
- Design complete states: loading, empty, error, success, validation, hover, focus, disabled, mobile.
- Accessibility: WCAG 2.1 AA, keyboard focus, color contrast, form labels.
- CSS: tokens at top of `application.css`, organized into tokens/base/layout/components/utilities/pages.
- Use system font stacks. Card radii ≤ 8px. No decorative blobs or hero illustrations.
- Partials for repeated UI. Rails helpers for links, forms, paths, assets.

## Hotwire (call `skill_view("rails-ai-hotwire")` for full reference)

- Turbo Drive: enabled by default. Handles navigation and form submissions without full page reloads.
- Turbo Frames: wrap sections in `<%= turbo_frame_tag :id do %>` for partial page updates. Use `src:` to lazy-load.
- Turbo Streams: broadcast updates with `<%= turbo_stream_from :channel %>` or respond with `.turbo_stream.erb` from controllers.
- Turbo Morph: page refreshes with DOM morphing for smooth updates.
- Stimulus: small controllers for client-side behavior. Use data attributes (`data-controller`, `data-action`, `data-target`). Keep controllers under 50 lines.

## Testing (call `skill_view("rails-ai-testing")` for full reference)

- Minitest with Rails default helpers. Test files in `test/`.
- Prefer integration tests: exercise routes, controllers, validations, persistence, redirects/renders, response bodies.
- Model tests: only for dense validations, scopes, calculations not proven by integration coverage.
- System tests: only when browser-level behavior is central.
- Static websites: no tests required. Web applications: tests required.
- Run `bin/rails test` before handoff. Run targeted tests while iterating.

## Useful Commands

```bash
cd /workspace/app
bin/rails routes
bin/rails generate model Name field:type
bin/rails generate controller Name action
bin/rails db:migrate
bin/rails db:rollback
bin/rails test
bin/rails test test/path/to/test.rb:line_number
nohup bin/rails server -b 0.0.0.0 -p 3000 > /tmp/rails-server.log 2>&1 &
```

## Security (call `skill_view("rails-ai-security")` for full reference)

- Strong parameters for all user input.
- Rails auto-escapes output in ERB `<%= %>` tags. Use `<%== %>` only when content is explicitly sanitized or trusted.
- SQL injection: use `?` placeholders or named bind variables. Never interpolate user input into SQL strings.
- CSRF: handled automatically by Rails with `protect_from_forgery`.
- Destructive actions behind non-GET verbs.
- File uploads: validate type, size, use direct upload when available.
- No secrets, credentials, or API keys in code.

## QA Evidence (call `skill_view("rails-qa-evidence")` for full reference)

When validating the app with a browser:
- Start server with nohup, capture PID, wait for readiness.
- Use agent-browser for screenshots and recordings.
- Test happy path, empty states, validation errors, success states, persistence after refresh.
- Save evidence to `/workspace/qa-runs/<timestamp>/`.
- Kill server when done.
