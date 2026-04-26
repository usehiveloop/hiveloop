package github

import (
	"strconv"

	"github.com/usehiveloop/hiveloop/internal/rag/acl"
	"github.com/usehiveloop/hiveloop/internal/rag/model"
)

// Group-id constructors run output through acl.BuildExtGroupName +
// acl.PrefixExternalGroup so the byte sequence matches the
// indexing-side ACL.
func collaboratorsGroupID(repoID int64) string {
	return acl.PrefixExternalGroup(
		acl.BuildExtGroupName(strconv.FormatInt(repoID, 10)+"_collaborators",
			model.DocumentSourceGithub))
}

func outsideCollaboratorsGroupID(repoID int64) string {
	return acl.PrefixExternalGroup(
		acl.BuildExtGroupName(strconv.FormatInt(repoID, 10)+"_outside_collaborators",
			model.DocumentSourceGithub))
}

func orgGroupID(orgID int64) string {
	return acl.PrefixExternalGroup(
		acl.BuildExtGroupName(strconv.FormatInt(orgID, 10)+"_organization",
			model.DocumentSourceGithub))
}

func teamGroupID(slug string) string {
	return acl.PrefixExternalGroup(
		acl.BuildExtGroupName(slug, model.DocumentSourceGithub))
}

// emailsFromUsers falls back to login when email is hidden so the
// junction-table upsert always has at least the username to match on.
func emailsFromUsers(users []GithubUser) []string {
	if len(users) == 0 {
		return nil
	}
	out := make([]string, 0, len(users))
	for _, u := range users {
		if u.Email != "" {
			out = append(out, u.Email)
		} else if u.Login != "" {
			out = append(out, u.Login)
		}
	}
	return out
}

func emailsFromMemberships(ms []GithubMembership) []string {
	if len(ms) == 0 {
		return nil
	}
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		if m.Email != "" {
			out = append(out, m.Email)
		} else if m.Login != "" {
			out = append(out, m.Login)
		}
	}
	return out
}

// isPublic checks both fields because older GitHub API versions
// populated only the `private` boolean; modern responses use visibility.
func isPublic(repo GithubRepo) bool {
	if repo.Visibility != "" {
		return repo.Visibility == "public"
	}
	return !repo.Private
}

func isInternal(repo GithubRepo) bool {
	return repo.Visibility == "internal"
}
