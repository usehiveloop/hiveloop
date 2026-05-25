package main

// githubResources returns the 10 GitHub resource definitions with action filtering.
func githubResources() map[string]ResourceFilterConfig {
	return map[string]ResourceFilterConfig{
		"repository": {
			DisplayName: "Repositories",
			Description: "GitHub repositories the AI can access",
			IDField:     "full_name",
			NameField:   "name",
			Icon:        "repo",
			RefBindings: map[string]string{
				"owner": "$refs.owner",
				"repo":  "$refs.repo",
			},
			ListAction: "/installation/repositories",
			ListRequestConfig: &RequestConfig{
				Method:       "GET",
				QueryParams:  map[string]string{"per_page": "100"},
				ResponsePath: "repositories",
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/collaborators",
				"/repos/{owner}/{repo}/topics",
				"/repos/{owner}/{repo}/forks",
				"/repos/{owner}/{repo}/contributors",
				"/repos/{owner}/{repo}/languages",
				"/repos/{owner}/{repo}/readme",
				"/repos/{owner}/{repo}/contents",
				"/repos/{owner}/{repo}/git",
				"/repos/{owner}/{repo}/commits",
				"/repos/{owner}/{repo}/comments",
				"/repos/{owner}/{repo}/stargazers",
				"/repos/{owner}/{repo}/compare",
				"/repos/{owner}/{repo}/merges",
				"/installation/repositories",
				"/user/repos",
				"/search/repositories",
				"/search/code",
				"/search/commits",
			},
			ExactPaths: []string{
				"/repos/{owner}/{repo}",
				"/repos/{owner}/{repo}/merge-upstream",
			},
		},
		"issue": {
			DisplayName: "Issues",
			Description: "GitHub issues within repositories",
			IDField:     "number",
			NameField:   "title",
			Icon:        "issue-opened",
			RefBindings: map[string]string{
				"owner":        "$refs.owner",
				"repo":         "$refs.repo",
				"issue_number": "$refs.issue_number",
			},
			ResourceKeyTemplate: "$refs.owner/$refs.repo#issue-$refs.issue_number",
			ListAction:          "/repos/{owner}/{repo}/issues",
			ListRequestConfig: &RequestConfig{
				Method: "GET",
				QueryParams: map[string]string{
					"per_page": "100",
					"state":    "all",
				},
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/issues",
				"/repos/{owner}/{repo}/assignees",
				"/search/issues",
			},
		},
		"pull_request": {
			DisplayName: "Pull Requests",
			Description: "GitHub pull requests for code review and merging",
			IDField:     "number",
			NameField:   "title",
			Icon:        "git-pull-request",
			RefBindings: map[string]string{
				"owner":       "$refs.owner",
				"repo":        "$refs.repo",
				"pull_number": "$refs.pull_number",
			},
			ResourceKeyTemplate: "$refs.owner/$refs.repo#pr-$refs.pull_number",
			ListAction:          "/repos/{owner}/{repo}/pulls",
			ListRequestConfig: &RequestConfig{
				Method: "GET",
				QueryParams: map[string]string{
					"per_page": "100",
					"state":    "all",
				},
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/pulls",
			},
		},
		"release": {
			DisplayName: "Releases",
			Description: "GitHub releases for versioning and distribution",
			IDField:     "id",
			NameField:   "tag_name",
			Icon:        "tag",
			RefBindings: map[string]string{
				"owner":      "$refs.owner",
				"repo":       "$refs.repo",
				"release_id": "$refs.release_id",
			},
			ResourceKeyTemplate: "$refs.owner/$refs.repo#release-$refs.release_id",
			ListAction:          "/repos/{owner}/{repo}/releases",
			ListRequestConfig: &RequestConfig{
				Method:      "GET",
				QueryParams: map[string]string{"per_page": "100"},
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/releases",
			},
		},
		"workflow": {
			DisplayName: "Workflows",
			Description: "GitHub Actions CI/CD workflows and runs",
			IDField:     "id",
			NameField:   "name",
			Icon:        "play",
			RefBindings: map[string]string{
				"owner":  "$refs.owner",
				"repo":   "$refs.repo",
				"run_id": "$refs.run_id",
			},
			// check_run.completed won't resolve this (it exposes check_run_id, not
			// run_id) — that's intentional. Check runs are one-shot status updates
			// and don't need cross-event continuation. Only workflow_run/workflow_job
			// events produce a stable key.
			ResourceKeyTemplate: "$refs.owner/$refs.repo#run-$refs.run_id",
			ListAction:          "/repos/{owner}/{repo}/actions/workflows",
			ListRequestConfig: &RequestConfig{
				Method:       "GET",
				QueryParams:  map[string]string{"per_page": "100"},
				ResponsePath: "workflows",
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/actions/workflows",
				"/repos/{owner}/{repo}/actions/runs",
				"/repos/{owner}/{repo}/actions/jobs",
				"/repos/{owner}/{repo}/actions/artifacts",
			},
		},
		"label": {
			DisplayName: "Labels",
			Description: "GitHub labels for categorizing issues and pull requests",
			IDField:     "id",
			NameField:   "name",
			Icon:        "tag",
			RefBindings: map[string]string{
				"owner": "$refs.owner",
				"repo":  "$refs.repo",
			},
			ListAction: "/repos/{owner}/{repo}/labels",
			ListRequestConfig: &RequestConfig{
				Method:      "GET",
				QueryParams: map[string]string{"per_page": "100"},
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/labels",
			},
		},
		"milestone": {
			DisplayName: "Milestones",
			Description: "GitHub milestones for tracking progress",
			IDField:     "number",
			NameField:   "title",
			Icon:        "milestone",
			RefBindings: map[string]string{
				"owner": "$refs.owner",
				"repo":  "$refs.repo",
			},
			ListAction: "/repos/{owner}/{repo}/milestones",
			ListRequestConfig: &RequestConfig{
				Method:      "GET",
				QueryParams: map[string]string{"per_page": "100"},
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/milestones",
			},
		},
		"branch": {
			DisplayName: "Branches",
			Description: "GitHub repository branches",
			IDField:     "name",
			NameField:   "name",
			Icon:        "git-branch",
			RefBindings: map[string]string{
				"owner":  "$refs.owner",
				"repo":   "$refs.repo",
				"branch": "$refs.branch_name",
			},
			ListAction: "/repos/{owner}/{repo}/branches",
			ListRequestConfig: &RequestConfig{
				Method:      "GET",
				QueryParams: map[string]string{"per_page": "100"},
			},
			PathPrefixes: []string{
				"/repos/{owner}/{repo}/branches",
			},
		},
		"organization": {
			DisplayName: "Organizations",
			Description: "GitHub organizations",
			IDField:     "login",
			NameField:   "login",
			Icon:        "organization",
			ListAction:  "/user/orgs",
			ListRequestConfig: &RequestConfig{
				Method:      "GET",
				QueryParams: map[string]string{"per_page": "100"},
			},
			PathPrefixes: []string{
				"/orgs/{org}/members",
				"/orgs/{org}/invitations",
				"/orgs/{org}/hooks",
				"/user/orgs",
			},
			ExactPaths: []string{
				"/orgs/{org}",
			},
		},
		"team": {
			DisplayName: "Teams",
			Description: "GitHub organization teams",
			IDField:     "slug",
			NameField:   "name",
			Icon:        "people",
			ListAction:  "/orgs/{org}/teams",
			ListRequestConfig: &RequestConfig{
				Method:      "GET",
				QueryParams: map[string]string{"per_page": "100"},
			},
			PathPrefixes: []string{
				"/orgs/{org}/teams",
			},
		},
	}
}
