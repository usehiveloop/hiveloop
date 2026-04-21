export interface RecipeTemplate {
  name: string
  description: string
  yaml: string
}

export const recipeTemplates: Record<string, RecipeTemplate[]> = {
  github: [
    {
      name: "Issue Triage Agent",
      description: "Automatically label, deduplicate, and respond to new issues",
      yaml: `conditions:
  match: all
  rules:
    - path: sender.login
      operator: not_one_of
      value: [dependabot[bot], renovate[bot]]

context:
  - as: issue
    action: issues_get
    ref: issue

  - as: labels
    action: issues_list_labels_for_repo
    ref: repository

  - as: similar
    action: search_issues_and_pull_requests
    params:
      q: "repo:$refs.repository is:issue state:open {{$issue.title}}"

instructions: |
  A new issue was opened in $refs.repository.

  ## New Issue
  **{{$issue.title}}** (#{{$refs.issue_number}})
  {{$issue.body}}

  ## Available Labels
  {{$labels}}

  ## Similar Open Issues
  {{$similar}}
`,
    },
    {
      name: "PR Code Review Agent",
      description: "Review pull request code changes and provide inline feedback",
      yaml: `conditions:
  match: all
  rules:
    - path: pull_request.draft
      operator: not_equals
      value: true

context:
  - as: pr
    action: pulls_get
    ref: pull_request

  - as: files
    action: pulls_list_files
    ref: pull_request

  - as: comments
    action: issues_list_comments
    ref: issue

  - as: rules
    action: repos_get_content
    ref: repository
    params:
      path: ".github/CONTRIBUTING.md"
    optional: true

instructions: |
  A pull request needs review in $refs.repository.

  ## Contributing Guidelines
  {{$rules}}

  ## Pull Request
  **{{$pr.title}}** (#{{$refs.pull_number}}) by {{$pr.user.login}}
  {{$pr.body}}

  ## Changed Files
  {{$files}}

  ## Existing Comments
  {{$comments}}
`,
    },
    {
      name: "PR Mention Responder",
      description: "Respond when @hiveloop is mentioned in a pull request comment",
      yaml: `conditions:
  match: all
  rules:
    - path: comment.body
      operator: contains
      value: "@hiveloop"
    - path: issue.pull_request
      operator: exists

context:
  - as: pr
    action: pulls_get
    ref: pull_request

  - as: files
    action: pulls_list_files
    ref: pull_request

  - as: comments
    action: issues_list_comments
    ref: issue

instructions: |
  You were mentioned in a pull request comment in $refs.repository.

  ## Comment
  {{$refs.sender}} said:
  > {{$comment.body}}

  ## Pull Request
  **{{$pr.title}}** (#{{$refs.pull_number}}) by {{$pr.user.login}}
  {{$pr.body}}

  ## Changed Files
  {{$files}}

  ## Conversation
  {{$comments}}
`,
    },
    {
      name: "Issue Mention Responder",
      description: "Respond when @hiveloop is mentioned in an issue comment",
      yaml: `conditions:
  match: all
  rules:
    - path: comment.body
      operator: contains
      value: "@hiveloop"
    - path: issue.pull_request
      operator: not_exists

context:
  - as: issue
    action: issues_get
    ref: issue

  - as: comments
    action: issues_list_comments
    ref: issue

  - as: labels
    action: issues_list_labels_for_repo
    ref: repository

instructions: |
  You were mentioned in an issue comment in $refs.repository.

  ## Comment
  {{$refs.sender}} said:
  > {{$comment.body}}

  ## Issue
  **{{$issue.title}}** (#{{$refs.issue_number}})
  {{$issue.body}}

  ## Conversation
  {{$comments}}

  ## Available Labels
  {{$labels}}
`,
    },
  ],
}

export function getBaseProvider(provider: string): string {
  const dashIndex = provider.indexOf("-")
  return dashIndex > 0 ? provider.slice(0, dashIndex) : provider
}
