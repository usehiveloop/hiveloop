---
name: rails-ai-testing
description: Use when testing Rails web applications with Rails default Minitest. Focus on integration tests that prove real user workflows, persistence, validation failures, redirects, authorization boundaries, and business behavior work end to end. Do not require tests before implementation.
---

# Rails Integration Testing with Minitest

<runtime-adaptation>
This vendored Rails-AI testing skill has been rewritten for the Hivy Rails no-build runtime.

Use Rails default Minitest and Rails test helpers. Do not require Superpowers, RSpec, Capybara-only flows, or user approval. For this eval, tests are required for Rails web applications and not required for simple static websites.
</runtime-adaptation>

<core-principle>
Tests exist to prove that the product behavior users depend on actually works.

Prioritize integration tests over isolated unit tests. A good test should exercise the real Rails route, controller action, model validations, database persistence, redirects/renders, flash/errors, and response body that make up a user-visible workflow.
</core-principle>

<when-to-use>
- Web applications with persisted user data, CRUD, dashboards, forms, filtering, search, authentication, authorization, or workflows
- New or changed Rails routes, controllers, models, validations, callbacks, jobs, mailers, or service objects
- Regression coverage for bugs and edge cases
- Reviewing whether a Rails app has meaningful behavior coverage
</when-to-use>

<do-not-use-for>
- Simple static websites with no persistence or mutable app behavior
- Pure visual polish where QA browser evidence is enough
</do-not-use-for>

<team-rules>
- Use Rails default Minitest only.
- Prefer `ActionDispatch::IntegrationTest` for app behavior.
- Write tests around outcomes, not implementation details.
- Do not prescribe development order.
- Do not add RSpec, FactoryBot, Capybara gems, WebMock, VCR, or extra testing gems unless the app already uses them and the user explicitly wants that direction.
- Do not make live external HTTP requests in tests. Stub at the boundary with Rails/Ruby primitives if an external boundary is unavoidable.
</team-rules>

<coverage-policy>
For web applications, cover the critical behavior through integration tests:

- Happy paths: create, update, delete, complete, search, filter, or submit the core resource/workflow.
- Failure paths: validation errors, missing required fields, invalid params, not-found records, and destructive actions.
- Persistence: assert database changes with `assert_difference` and verify records contain expected values.
- Routing and response behavior: assert status, redirects, rendered content, flash messages, and important links/forms.
- Business rules: assert the workflow result that proves the rule, not just a private method return.
- Security boundaries: assert unsafe params are ignored, destructive actions are non-GET, and scoped data is not exposed.

Add model tests only when a domain rule is dense enough that integration coverage alone would be unclear or brittle. Keep model tests focused on validations, associations, scopes, and calculations that represent real business rules.

Add system tests only when browser-only behavior is central to the feature and local browser support is available. Otherwise, rely on integration tests plus QA browser evidence.
</coverage-policy>

<verification-checklist>
Before handoff on a web app:

- Integration tests exercise the user's primary workflows.
- Tests assert database persistence or mutation where relevant.
- Tests cover at least one important failure path for each form/workflow.
- Tests cover business behavior at the boundary a user or controller hits.
- Static sites did not receive unnecessary tests.
- `bin/rails test` or the narrow relevant Rails test command was run and reported.
</verification-checklist>

---

## Integration Test Template

Use `ActionDispatch::IntegrationTest` for full request/response behavior.

```ruby
# test/integration/habits_flow_test.rb
require "test_helper"

class HabitsFlowTest < ActionDispatch::IntegrationTest
  test "creates a habit and shows it on the dashboard" do
    assert_difference "Habit.count", 1 do
      post habits_path, params: {
        habit: {
          name: "Drink water",
          cadence: "daily"
        }
      }
    end

    habit = Habit.order(:created_at).last
    assert_redirected_to habit_path(habit)
    follow_redirect!

    assert_response :success
    assert_select "h1", text: "Drink water"
    assert_select ".flash", text: /created/i
  end

  test "rejects a habit without a name" do
    assert_no_difference "Habit.count" do
      post habits_path, params: {
        habit: {
          name: ""
        }
      }
    end

    assert_response :unprocessable_entity
    assert_select ".error", text: /name/i
  end
end
```

## CRUD Coverage Shape

For a resource-backed workflow, prefer one integration file that covers the behavior users care about:

```ruby
require "test_helper"

class ProjectsFlowTest < ActionDispatch::IntegrationTest
  setup do
    @project = projects(:website_redesign)
  end

  test "lists existing projects" do
    get projects_path

    assert_response :success
    assert_select "a[href=?]", project_path(@project), text: @project.name
  end

  test "updates a project" do
    patch project_path(@project), params: {
      project: {
        name: "Launch site"
      }
    }

    assert_redirected_to project_path(@project)
    assert_equal "Launch site", @project.reload.name
  end

  test "deletes a project" do
    assert_difference "Project.count", -1 do
      delete project_path(@project)
    end

    assert_redirected_to projects_path
    follow_redirect!
    assert_select "body", text: /deleted/i
  end
end
```

## Business Rule Coverage

Test business logic through the workflow whenever practical.

```ruby
test "checking in today increments the visible streak" do
  habit = habits(:daily_walk)

  assert_difference "CheckIn.count", 1 do
    post habit_check_ins_path(habit), params: {
      check_in: {
        completed_on: Date.current
      }
    }
  end

  assert_redirected_to habits_path
  follow_redirect!

  assert_select "[data-habit-id='#{habit.id}'] .streak", text: /1 day/
  assert_equal 1, habit.reload.current_streak
end
```

If the calculation has many edge cases, add a focused model test in addition to the integration test:

```ruby
require "test_helper"

class HabitTest < ActiveSupport::TestCase
  test "current streak counts consecutive completed days ending today" do
    habit = habits(:daily_walk)
    habit.check_ins.create!(completed_on: Date.current)
    habit.check_ins.create!(completed_on: Date.yesterday)

    assert_equal 2, habit.current_streak
  end
end
```

## Form Failure Coverage

Every important form should prove that invalid input preserves data safety and shows a useful error.

```ruby
test "does not update with invalid data" do
  original_name = @project.name

  patch project_path(@project), params: {
    project: {
      name: ""
    }
  }

  assert_response :unprocessable_entity
  assert_equal original_name, @project.reload.name
  assert_select ".error", text: /name/i
end
```

## Strong Parameter Coverage

For user-controlled forms, prove unsafe params are ignored.

```ruby
test "does not allow protected fields through params" do
  user = users(:member)

  patch user_path(user), params: {
    user: {
      name: "New Name",
      admin: true
    }
  }

  assert_equal "New Name", user.reload.name
  assert_not user.admin?
end
```

## Fixtures

Use Rails fixtures for stable test records.

```yaml
# test/fixtures/habits.yml
daily_walk:
  name: Daily walk
  cadence: daily

reading:
  name: Read 20 pages
  cadence: daily
```

Keep fixtures realistic and small. Prefer naming records after their role in the test instead of `one` and `two`.

## Commands

Run the narrowest meaningful command while iterating:

```bash
bin/rails test test/integration/habits_flow_test.rb
bin/rails test test/models/habit_test.rb
```

Before handoff for a web app, run:

```bash
bin/rails test
```

Report exact commands and pass/fail results.
