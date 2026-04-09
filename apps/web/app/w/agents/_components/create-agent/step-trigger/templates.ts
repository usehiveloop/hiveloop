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

  Please triage this issue:
  1. Add the most appropriate labels from the available list
  2. If this is a duplicate of a similar issue, comment explaining which issue it duplicates and close it
  3. If the issue lacks steps to reproduce or clear description, comment asking for more details
  4. If the issue is clear and actionable, add a comment acknowledging it
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

  Please review the code changes:
  1. Check for bugs, security issues, and performance problems
  2. Verify the code follows the contributing guidelines
  3. Look for missing error handling or edge cases
  4. Post specific, actionable feedback as review comments
  5. If everything looks good, approve the pull request
`,
    },
  ],
}

export function getBaseProvider(provider: string): string {
  const dashIndex = provider.indexOf("-")
  return dashIndex > 0 ? provider.slice(0, dashIndex) : provider
}
