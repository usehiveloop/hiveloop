---
name: git-github
description: Use whenever you need to perform common human git or GitHub activities from the CLI — creating branches, writing commits, opening pull requests (including PRs with screenshots, GIFs, or video demos in the description), commenting on PRs or issues, reacting (👍 / 🚀 / 👀), adding labels, requesting reviews, merging, checking status, fetching diffs. Always inspect the repo first to discover its branch-naming, commit, label, and PR conventions before acting. PR images and demo media are always uploaded through the `asset-uploads` skill, never gists or base64 — load that skill before composing a PR body that includes any asset. Triggers include: "open a PR", "create a pull request", "comment on PR", "react to that comment", "label this PR", "create a branch", "commit this", "follow the commit convention", "attach a screenshot", "embed an image in the PR", "include a demo video", "before/after screenshots", "request a review", "merge this", "draft PR".
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

If the repo has a PR template (`.github/PULL_REQUEST_TEMPLATE.md`), `gh pr create` only loads it in **interactive** mode. When you pass `--body` or `--body-file` non-interactively, the template is bypassed — so read it yourself first and incorporate its structure into your `--body`/`--body-file` content. Don't strip the template's sections.

```bash
test -f .github/PULL_REQUEST_TEMPLATE.md && cat .github/PULL_REQUEST_TEMPLATE.md
```

### 4b. PR with uploaded images / screenshots

**This is the only supported way to host an image in a PR description.** Do not commit screenshots into the project repo, do not paste data-URIs into markdown, do not create a gist, do not call GitHub's `user-attachments` endpoint, do not upload to imgur or any other host. All of these are wrong.

Every PR image, video, GIF, before/after screenshot, demo recording, etc. must be uploaded through our **assets drive** so the URL is stable, owned by us, and tied to the current employee/task asset folder.

**Load the `asset-uploads` skill before generating the PR body.** It documents the exact `curl` invocation, the env vars (`HIVELOOP_DRIVE_UPLOAD_URL`, `UPLOAD_BEARER`), the response shape, and the conventions for organising assets into folders. Do not try to reconstruct the upload protocol from memory — load the skill, follow it.

Workflow:

```bash
# 1. Load the asset-uploads skill (don't skip this — the curl
#    incantation, headers, and URL shape are all in there).

# 2. Upload the screenshot using that skill's curl invocation. It returns
#    a JSON response with an `asset_url` field. Capture it:
URL=$(
  curl -fsS -X PUT \
    -H "Authorization: Bearer $UPLOAD_BEARER" \
    -H "Content-Type: image/png" \
    --upload-file ./screenshot.png \
    "$HIVELOOP_DRIVE_UPLOAD_URL/pr/screenshot.png" \
  | jq -r .asset_url
)

# 3. Embed the URL in the PR body. Use a descriptive folder so future
#    readers can tell what's what (pr/, demos/, before-after/, etc).
gh pr create --title "..." --body "$(cat <<EOF
## Summary
Before/after:

![screenshot]($URL)
EOF
)"
```

For multiple images or a before/after pair, run the upload step once per file and paste each URL. For animated demos, upload an `.mp4` or `.webm` and embed it the same way (GitHub renders video URLs inline).

If you ever find yourself reaching for `gh gist`, `git push` to a gist repo, base64 inside markdown, or a third-party host: stop. Load `asset-uploads` skill and use it.

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

A PR review is a single submission that bundles an overall verdict (approve / request changes / comment), an optional summary body, and zero or more **line-by-line inline comments** anchored to specific lines of the diff. Post the whole thing in one request to `pulls/<number>/reviews` so the inline comments appear as part of the review (not as orphaned standalone comments).

For a review with no inline notes, `gh pr review` is the shortcut:

```bash
gh pr review <number> --approve --body "LGTM"
gh pr review <number> --request-changes --body "See inline notes."
gh pr review <number> --comment --body "One question below."
```

For a review **with line-by-line inline comments**, build a JSON payload and POST it to the reviews endpoint. Each entry in `comments[]` is anchored to a file + line in the diff:

```bash
cat > /tmp/review.json <<'JSON'
{
  "event": "REQUEST_CHANGES",
  "body": "A few inline notes — see comments on the diff.",
  "comments": [
    {
      "path": "internal/charges/handler.go",
      "line": 128,
      "side": "RIGHT",
      "body": "Consider extracting this to a helper — it's duplicated in `refund.go:84`."
    },
    {
      "path": "internal/charges/handler.go",
      "start_line": 142,
      "start_side": "RIGHT",
      "line": 150,
      "side": "RIGHT",
      "body": "This whole block should be inside the transaction opened on line 120."
    },
    {
      "path": "internal/charges/handler_test.go",
      "line": 47,
      "side": "RIGHT",
      "body": "Missing a test for the `ErrAlreadyRefunded` branch."
    }
  ]
}
JSON

gh api -X POST "repos/:owner/:repo/pulls/<number>/reviews" --input /tmp/review.json
```

Field reference for each entry in `comments[]`:

- `path` — file path as it appears in the diff (required).
- `line` — line number in the file the comment anchors to (required for single-line; for multi-line, this is the **last** line of the range).
- `side` — `RIGHT` for the new version (additions / context), `LEFT` for the old version (deletions). Default `RIGHT`.
- `start_line` + `start_side` — only for multi-line comments; the first line of the range. Omit for single-line.
- `body` — the comment text (Markdown).

Top-level fields:

- `event` — `APPROVE`, `REQUEST_CHANGES`, `COMMENT`, or omit/`PENDING` to draft without submitting.
- `body` — overall review summary shown above the inline comments. Optional.
- `commit_id` — optional; pin the review to a specific SHA. Defaults to the PR head.

Common gotchas:

- The `line` must be a line that appears in the PR's diff (added, removed, or context within a hunk). Anchoring to a line outside the diff returns `422 Unprocessable Entity` ("Line could not be resolved").
- Use `LEFT` only when commenting on a removed or pre-change line; new code is always `RIGHT`.
- For multi-line comments, `start_line` must come **before** `line` in the file, and both `side` values must match unless you're spanning a deletion.
- GitHub forbids `APPROVE` and `REQUEST_CHANGES` on your **own** PR — both return `422`. Use `COMMENT` for self-reviews.

To post a one-off inline comment without composing a review summary, hit the comments endpoint directly. (GitHub still wraps it in an implicit empty-body review under the hood, so it shows up as its own review thread on the PR.)

```bash
gh api -X POST "repos/:owner/:repo/pulls/<number>/comments" \
  -f body="Drive-by note: typo in the log message." \
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
| Committing screenshots into the project repo to embed in a PR | Upload them via the `asset-uploads` skill — that's the only supported host |
| Reaching for gists, imgur, base64 data-URIs, or `gh api user-attachments` for PR images | All wrong. Load `asset-uploads` and use it; it returns a stable file URL you can paste into the PR body |
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
