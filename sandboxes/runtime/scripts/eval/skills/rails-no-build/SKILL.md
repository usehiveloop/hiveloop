---
name: rails-no-build
description: Rails 8.1 no-build implementation guide for apps that must use Rails defaults, SQLite, ERB, Propshaft, Import Maps, Turbo, Stimulus, plain CSS, and Minitest. Use whenever building or editing Rails app code in this eval.
category: rails
tags: [rails, no-build, hotwire, sqlite, minitest]
---

# Rails No-Build

Use this skill when implementing application behavior in the generated Rails app at `/workspace/app`.

## Stack Contract

Use Rails 8.1.x defaults:

- ERB, layouts, partials, helpers, forms, and Rails route helpers
- Propshaft-managed assets
- Import Maps for JavaScript
- Turbo and Stimulus for progressive UI
- SQLite for development and test
- Minitest with Rails default test helpers

Do not add React, Vue, Svelte, Vite, Webpack, esbuild, Bun, Tailwind, npm package installs, PostCSS, Sass build steps, or client-side SPA routing.

## Rails Shape

- Prefer RESTful routes and resourceful controllers.
- Keep controllers thin and move reusable domain rules into models or small service objects.
- Use Active Record validations, associations, scopes, and database indexes intentionally.
- Use Rails forms and server-rendered validation errors.
- Use Turbo Frames/Streams when the UI benefits from partial page updates.
- Use Stimulus only for small browser-side behaviors that HTML/Turbo cannot express cleanly.

## Static Site vs Web App

Static site:

- Use a controller or route-backed static pages.
- Keep the work in layouts, views, partials, and CSS.
- Do not add tests unless the request grows into application behavior.

Web application:

- Add tests for meaningful behavior.
- Cover models, integration flows, and important system behavior when practical.
- Run `bin/rails test` before handoff when feasible.

## Useful Commands

```bash
cd /workspace/app
bin/rails routes
bin/rails generate model Name field:type
bin/rails generate controller Name action
bin/rails db:migrate
bin/rails test
bin/rails server -b 0.0.0.0 -p 3000
```

## Source Notes

Adapted from official Rails 8.1 guides and Rails no-build conventions: Import Maps, Hotwire, Propshaft, SQLite defaults, and Rails-native testing.
