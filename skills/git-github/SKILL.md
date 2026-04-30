---
name: git-github
description: Use whenever you need to perform common human git or GitHub activities from the CLI — creating branches, writing commits, opening pull requests (including PRs with screenshots / uploaded images), commenting on PRs or issues, reacting (👍 / 🚀 / 👀), adding labels, requesting reviews, merging, checking status, fetching diffs. Always inspect the repo first to discover its branch-naming, commit, label, and PR conventions before acting. Triggers include: "open a PR", "create a pull request", "comment on PR", "react to that comment", "label this PR", "create a branch", "commit this", "follow the commit convention", "attach a screenshot", "request a review", "merge this", "draft PR".
---

# Git + GitHub workflows from the CLI

Practical playbook for the most common day-to-day human activities on a git repo and on GitHub via the `gh` CLI.

The single most important rule: **inspect the repo's existing conventions before you act**. Branch names, commit messages, PR titles, labels, and review etiquette differ per repo and per team. Match what's already there — don't invent a new style.

## Prerequisites

```bash
git --version
gh --version
gh auth status   # must say "Logged in"
```

If `gh auth status` fails, ask the user to run `gh auth login` themselves — do not attempt browser auth on their behalf.

---

## 1. Discover repo conventions FIRST

Run these before creating branches, commits, or PRs. They take seconds and save you from getting your work bounced in review.

### 1a. Branch-naming convention

```bash
# Recent branches that have been merged into the default branch
git for-each-ref --sort=-committerdate --count=30 \
  --format='%(refname:short)' refs/remotes/origin | grep -v HEAD

# Or pull request branches
gh pr list --state all --limit 30 --json headRefName -q '.[].headRefName'
```

Look for the pattern: `feat/...`, `feature/...`, `fix/...`, `chore/...`, `<user>/<topic>`, `<ticket-id>-...`, `bahdcoder/foo`, etc. Match whatever dominates the last 20–30 branches.

If the repo has CONTRIBUTING.md, `.github/CONTRIBUTING.md`, or a `docs/` folder, grep it:

```bash
grep -riE "branch (naming|name|convention)" CONTRIBUTING.md .github/ docs/ 2>/dev/null
```

### 1b. Commit-message convention

```bash
# Last 30 commits on the default branch — strongest signal
git log --no-merges -n 30 --pretty=format:'%s' origin/HEAD

# Look for tooling that enforces a convention
ls -a | grep -iE 'commitlint|husky|lefthook|gitmessage|gitlint'
test -f .gitmessage && cat .gitmessage
test -f commitlint.config.js && cat commitlint.config.js
test -f .husky/commit-msg && cat .husky/commit-msg
```

Common patterns to detect:
- **Conventional Commits** — `feat:`, `fix:`, `chore:`, `feat(scope): …`. Look for any colon-prefixed type at the start of subjects.
- **Ticketed** — `[ABC-123] …` or `ABC-123: …`.
- **Imperative free-form** — short imperative subject, no prefix.
- **Gitmoji** — only if you literally see emojis in `git log`.

If the project's contribution guide or CHANGELOG.md says something explicit, that wins.

### 1c. PR title / body conventions

```bash
# Recent merged PRs — the canonical examples
gh pr list --state merged --limit 20 --json number,title,body,labels
gh pr view <recent-PR-number>          # see how a real PR is structured

# Any PR template?
ls .github/PULL_REQUEST_TEMPLATE* .github/pull_request_template* 2>/dev/null
cat .github/PULL_REQUEST_TEMPLATE.md 2>/dev/null
```

If a template exists, **use it verbatim** — fill the sections, don't replace them.

### 1d. Labels in use

```bash
gh label list --limit 100
gh pr list --state all --limit 30 --json labels -q '.[].labels[].name' | sort | uniq -c | sort -rn
```

Pick labels that already exist; do not invent new ones unless asked.

---

## 2. Branching

```bash
# Always start from a fresh default branch
git fetch origin
DEFAULT=$(gh repo view --json defaultBranchRef -q .defaultBranchRef.name)
git switch "$DEFAULT" && git pull --ff-only

# Create the branch using the convention you discovered
git switch -c feat/short-topic        # adjust prefix to match repo
```

Tips:
- Keep names lowercase, kebab-case, ≤ 50 chars.
- Include ticket IDs if the repo uses them (`ENG-1234-fix-foo`).
- If unsure between two prefixes, pick the one that appears more often in `git log` of the last month.

---

## 3. Committing

### Stage with intent

```bash
git status
git diff                 # unstaged
git diff --staged        # staged
git add path/to/file     # prefer specific paths over `git add -A`
```

Never `git add -A` blindly — it can pull in `.env`, build artifacts, screenshots, or unrelated edits.

### Write the message in the repo's style

**Keep the subject line short and sweet — under 10 words. Always.** This is non-negotiable regardless of repo style. The subject is a headline, not a summary. If there is more to say, put it in the body separated by a blank line.

```
# Good
feat(api): add idempotency key to /charges

# Bad — subject doing the body's job
feat(api): add idempotency key to /charges so retried POSTs don't double-bill customers when the network drops mid-request
```

After detecting the convention (section 1b), follow it:

```bash
# Conventional Commits example
git commit -m "feat(api): add idempotency key to /charges"

# Ticketed example
git commit -m "ENG-1234: add idempotency key to /charges"
```

For multi-paragraph bodies, use a heredoc so formatting is preserved:

```bash
git commit -m "$(cat <<'EOF'
feat(api): add idempotency key to /charges

Why: duplicate POSTs from retrying clients were creating double charges.
The key is hashed (sha256) before being stored alongside the charge row.

Refs: ENG-1234
EOF
)"
```

### If a hook fails

Hooks run on `git commit` (lint, typecheck, tests). If one fails, **fix the underlying problem and create a new commit** — never re-run with `--no-verify` or `--amend` to dodge the hook unless the user explicitly asks.

### Pushing

```bash
git push -u origin HEAD            # first push of a new branch
git push                           # subsequent pushes
```

Never force-push to `main`/`master`/`develop`. If you must rewrite history on a feature branch you own, prefer `git push --force-with-lease` over `--force`.

---

## 4. Pull requests

### 4a. Create a PR

Before writing the title and body, look at the diff against the base branch:

```bash
BASE=$(gh repo view --json defaultBranchRef -q .defaultBranchRef.name)
git log --oneline "origin/$BASE"..HEAD
git diff "origin/$BASE"...HEAD --stat
```

Then open it:

```bash
gh pr create \
  --base "$BASE" \
  --title "feat(api): add idempotency key to /charges" \
  --body "$(cat <<'EOF'
## Summary
- Adds an `Idempotency-Key` header to `POST /charges`.
- Stores a sha256 of the key on the `charges` row; conflicts return the prior result.

## Test plan
- [x] Unit: `go test ./internal/charges/...`
- [x] Integration: replay the same key twice; second call returns 200 with the original body.

Refs: ENG-1234
EOF
)"
```

Useful flags:
- `--draft` — open as draft.
- `--reviewer user1,user2,@org/team` — request reviewers.
- `--assignee @me` — self-assign.
- `--label "bug,needs-review"` — only labels that already exist (see 1d).
- `--body-file pr.md` — when the body is large; easier than escaping.
- `--web` — print the URL and open in browser instead of creating via API (use only if the user asks).

If the repo has a PR template, `gh pr create` will pre-fill it. Read what `gh` opens, fill the placeholders, don't strip the template structure.

### 4b. PR with uploaded images / screenshots

`gh` does not support attaching binary images directly to a PR body. Two reliable approaches:

**Option A — gist-hosted (recommended for ad-hoc screenshots).** Upload the image to a gist, then reference its raw URL in markdown.

```bash
# Create a public gist with the image; gh prints the gist URL
GIST_URL=$(gh gist create --public /tmp/screenshot.png)
# Convert to a raw URL gh can render in markdown
GIST_ID=$(basename "$GIST_URL")
RAW_URL="https://gist.githubusercontent.com/$(gh api user -q .login)/$GIST_ID/raw/screenshot.png"

gh pr create --title "..." --body "$(cat <<EOF
## Summary
Before/after screenshot:

![screenshot]($RAW_URL)
EOF
)"
```

**Option B — commit the asset.** If the repo has a `docs/` or `assets/` folder for screenshots, drop the image there in a separate commit and reference it via the GitHub blob raw URL:

```bash
mkdir -p docs/assets/pr-1234
cp /tmp/screenshot.png docs/assets/pr-1234/before.png
git add docs/assets/pr-1234/before.png
git commit -m "docs: add screenshot for PR-1234"
git push
# In PR body:
# ![before](https://github.com/<owner>/<repo>/raw/<branch>/docs/assets/pr-1234/before.png)
```

Use Option A for transient screenshots (review only); use Option B if the screenshot belongs in the repo permanently.

Do **not** try to call GitHub's `user-attachments` upload endpoint from `gh api` — it's an undocumented browser-only API and breaks unpredictably.

### 4c. Update an existing PR

```bash
gh pr edit <number> --title "..." --add-label "ready-for-review" --remove-label "wip"
gh pr edit <number> --body-file pr-updated.md
gh pr ready <number>           # mark a draft as ready
```

---

## 5. Comments

### Top-level PR / issue comment

```bash
gh pr comment <number> --body "Pushed the requested fix in $(git rev-parse --short HEAD). Ready for another look."
gh issue comment <number> --body-file followup.md
```

### Inline review comment / leave a review

```bash
# Approve, request changes, or just comment
gh pr review <number> --approve --body "LGTM"
gh pr review <number> --request-changes --body "See inline notes."
gh pr review <number> --comment --body "One question below."
```

For per-line inline review threads, drive `gh api` directly:

```bash
gh api -X POST "repos/:owner/:repo/pulls/<number>/comments" \
  -f body="Consider extracting this to a helper." \
  -f commit_id="$(git rev-parse HEAD)" \
  -f path="internal/charges/handler.go" \
  -F line=128 \
  -f side="RIGHT"
```

---

## 6. Reactions

GitHub reactions (👍 👎 😄 🎉 😕 ❤️ 🚀 👀) are added via the Reactions API. Content values: `+1`, `-1`, `laugh`, `hooray`, `confused`, `heart`, `rocket`, `eyes`.

```bash
# React to a PR/issue (a PR is an issue under the hood)
gh api -X POST "repos/:owner/:repo/issues/<number>/reactions" -f content="rocket"

# React to a regular issue/PR comment
gh api -X POST "repos/:owner/:repo/issues/comments/<comment-id>/reactions" -f content="+1"

# React to an inline pull-request review comment
gh api -X POST "repos/:owner/:repo/pulls/comments/<comment-id>/reactions" -f content="eyes"
```

Find the comment ID:

```bash
gh api "repos/:owner/:repo/issues/<number>/comments" --jq '.[] | {id, user: .user.login, body: (.body[0:80])}'
```

---

## 7. Labels

```bash
gh label list --limit 100                          # what already exists
gh pr edit <number> --add-label "bug,needs-review"
gh pr edit <number> --remove-label "wip"
gh issue edit <number> --add-label "good first issue"
```

Only create new labels if the user explicitly asks:

```bash
gh label create "needs-design" --color "FBCA04" --description "Blocked on design input"
```

---

## 8. Reviews and merging

```bash
gh pr list --search "review-requested:@me"
gh pr checks <number>                              # CI status
gh pr view <number> --json reviews,statusCheckRollup,mergeStateStatus

# Merge — pick the strategy the repo actually uses
gh pr merge <number> --squash --delete-branch
gh pr merge <number> --merge --delete-branch
gh pr merge <number> --rebase --delete-branch
```

Detect the merge strategy in use:

```bash
# Look at recent merged PRs — squash merges show one commit, merge commits show "Merge pull request #..."
git log --oneline -n 20 origin/HEAD
gh api "repos/:owner/:repo" --jq '{allow_squash: .allow_squash_merge, allow_merge: .allow_merge_commit, allow_rebase: .allow_rebase_merge}'
```

Never merge a PR you didn't author without explicit approval from the user.

---

## 9. Checking out / reviewing someone else's PR

```bash
gh pr checkout <number>             # creates a local branch tracking the PR
gh pr diff <number>
gh pr view <number> --comments
```

---

## 10. Common pitfalls

| Pitfall | Fix |
|---|---|
| Inventing a branch prefix the repo doesn't use | Run `gh pr list --json headRefName` first; match the dominant pattern |
| Writing a Conventional Commit in a repo that uses free-form messages (or vice versa) | Read the last 30 commits on the default branch before composing the message |
| Replacing the PR template with your own structure | Fill the template's sections; don't strip them |
| `git add -A` sweeping in `.env`, `node_modules/`, screenshots | Add specific paths; verify with `git status` before committing |
| Subject line longer than 10 words / stuffing the whole rationale into the subject | Keep the subject under 10 words; move the "why" and details into the body |
| `--no-verify` to skip a failing hook | Fix the underlying issue and commit again — never bypass without explicit user approval |
| Force-pushing to a shared branch | Use `--force-with-lease` on your own branch; never force-push `main` |
| Inventing labels that don't exist | `gh label list` first; only create new labels if asked |
| Trying to attach binary images to `gh pr create --body` | Host via gist (`gh gist create`) or commit to repo, then reference the raw URL |
| Merging without checking CI | `gh pr checks <n>` and `mergeStateStatus` before `gh pr merge` |
| Approving / merging someone else's PR without being asked | Don't. Ask the user first |

---

## Quick command index

```bash
# Auth & repo info
gh auth status
gh repo view --json defaultBranchRef,nameWithOwner

# Branch
git fetch origin && git switch -c feat/x origin/main

# Commit
git add path && git commit -m "..."
git push -u origin HEAD

# PR
gh pr create --title "..." --body-file pr.md --reviewer user1
gh pr edit <n> --add-label "..." --remove-label "..."
gh pr ready <n>

# Comments / reviews
gh pr comment <n> --body "..."
gh pr review <n> --approve --body "..."

# Reactions
gh api -X POST "repos/:owner/:repo/issues/<n>/reactions" -f content="rocket"

# Labels
gh label list
gh pr edit <n> --add-label "bug"

# Merge
gh pr checks <n>
gh pr merge <n> --squash --delete-branch
```
