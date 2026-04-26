// PermSync: visibility-based ACL + group enumeration.
//
// Visibility translation (utils.py:28-33):
//
//   public   → ExternalAccess{IsPublic: true}, no groups.
//   private  → ExternalAccess{IsPublic: false},
//              groups = {collaborators, outside_collaborators, all team-slugs}
//   internal → ExternalAccess{IsPublic: false},
//              groups = {<org_id>_organization}
//
// Onyx analog (split):
//   - doc_sync.py:34-142  — per-doc ExternalAccess
//   - group_sync.py:14-51 — per-source ExternalGroup catalog
//   - utils.py:249-277    — group-id forms
//
// We unify them: SyncDocPermissions yields one DocExternalAccess per
// repo-doc; SyncExternalGroups yields one ExternalGroup per
// collaborators / outside-collaborators / team / org-membership group.
package github

import (
	"context"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// mapVisibility translates a GithubRepo into ExternalAccess. The group
// IDs produced here are the same byte sequences that SyncExternalGroups
// emits for the same repo — both sides go through acl.PrefixExternalGroup
// + acl.BuildExtGroupName to pin the lowercase + namespacing invariant.
func mapVisibility(repo GithubRepo) *interfaces.ExternalAccess {
	if isPublic(repo) {
		return &interfaces.ExternalAccess{IsPublic: true}
	}
	if isInternal(repo) {
		return &interfaces.ExternalAccess{
			ExternalUserGroupIDs: []string{orgGroupID(repo.Owner.ID)},
		}
	}
	// private: collaborators + outside-collaborators + (every team-slug
	// resolves dynamically when SyncExternalGroups walks teams; for the
	// per-doc ACL we inject the static group IDs that perm-sync will
	// also publish).
	return &interfaces.ExternalAccess{
		ExternalUserGroupIDs: []string{
			collaboratorsGroupID(repo.ID),
			outsideCollaboratorsGroupID(repo.ID),
		},
	}
}

// SyncDocPermissions streams one DocExternalAccess per
// repo-document. The pattern mirrors fetch_prs.go / fetch_issues.go but
// emits SlimDocument-shaped access updates rather than full Documents.
//
// Onyx analog: doc_sync.py:34-142 — same per-repo loop, same visibility
// branch. We unify PRs + Issues here: every doc in the repo shares the
// same repo-level ExternalAccess.
func (c *GithubConnector) SyncDocPermissions(
	ctx context.Context, _ interfaces.Source,
) (<-chan interfaces.DocExternalAccessOrFailure, error) {
	out := make(chan interfaces.DocExternalAccessOrFailure, c.channelBuf)
	go func() {
		defer close(out)
		for _, full := range c.repoFullNames() {
			repo, err := c.client.getRepo(ctx, full)
			if err != nil {
				out <- interfaces.NewAccessFailure(entityFailure(full, "github: get repo", err))
				continue
			}
			access := mapVisibility(repo)
			c.streamRepoDocAccess(ctx, full, access, out)
		}
	}()
	return out, nil
}

// streamRepoDocAccess walks PRs + Issues for a single repo and emits
// DocExternalAccess for each. We reuse the listing endpoints because we
// need doc IDs anyway — the slim variant exists for prune diffing, but
// for perm-sync we want the same ACL applied to every PR/Issue regardless
// of state.
func (c *GithubConnector) streamRepoDocAccess(
	ctx context.Context, fullName string, access *interfaces.ExternalAccess,
	out chan<- interfaces.DocExternalAccessOrFailure,
) {
	if c.cfg.IncludePRs {
		page := 1
		for {
			prs, next, err := c.client.listPullRequestsPage(ctx, fullName, "all", page)
			if err != nil {
				out <- interfaces.NewAccessFailure(entityFailure(fullName, "github: list PRs", err))
				break
			}
			for _, pr := range prs {
				out <- interfaces.NewAccessResult(&interfaces.DocExternalAccess{
					DocID:          docIDForPR(fullName, pr),
					ExternalAccess: access,
				})
			}
			if next == 0 {
				break
			}
			page = next
		}
	}
	if c.cfg.IncludeIssues {
		page := 1
		for {
			issues, next, err := c.client.listIssuesPage(ctx, fullName, "all", page)
			if err != nil {
				out <- interfaces.NewAccessFailure(entityFailure(fullName, "github: list issues", err))
				break
			}
			for _, issue := range issues {
				if issue.PullRequest != nil {
					continue
				}
				out <- interfaces.NewAccessResult(&interfaces.DocExternalAccess{
					DocID:          docIDForIssue(fullName, issue),
					ExternalAccess: access,
				})
			}
			if next == 0 {
				break
			}
			page = next
		}
	}
}

// SyncExternalGroups streams the per-source group catalog. Per repo:
//
//   public   → no groups (everyone sees it; nothing to enumerate).
//   private  → collaborators, outside-collaborators, team(s).
//   internal → org-membership.
//
// Onyx analog: group_sync.py:14-51.
func (c *GithubConnector) SyncExternalGroups(
	ctx context.Context, _ interfaces.Source,
) (<-chan interfaces.ExternalGroupOrFailure, error) {
	out := make(chan interfaces.ExternalGroupOrFailure, c.channelBuf)
	go func() {
		defer close(out)
		for _, full := range c.repoFullNames() {
			repo, err := c.client.getRepo(ctx, full)
			if err != nil {
				out <- interfaces.NewGroupFailure(entityFailure(full, "github: get repo", err))
				continue
			}
			c.enumerateGroups(ctx, repo, out)
		}
	}()
	return out, nil
}

// enumerateGroups emits the right group set for the given repo. Helpers
// that fetch member lists wrap their own error path so a partial failure
// (e.g. one team's members fail) doesn't kill the whole sync.
func (c *GithubConnector) enumerateGroups(
	ctx context.Context, repo GithubRepo, out chan<- interfaces.ExternalGroupOrFailure,
) {
	if isPublic(repo) {
		return
	}
	if isInternal(repo) {
		members, err := c.client.listOrgMembers(ctx, repo.Owner.Login)
		if err != nil {
			out <- interfaces.NewGroupFailure(entityFailure(repo.FullName, "github: list org members", err))
			return
		}
		out <- interfaces.NewGroupResult(&interfaces.ExternalGroup{
			GroupID:      orgGroupID(repo.Owner.ID),
			DisplayName:  repo.Owner.Login + " organization",
			MemberEmails: emailsFromMemberships(members),
		})
		return
	}
	// private:
	c.emitCollaboratorGroups(ctx, repo, out)
	c.emitTeamGroups(ctx, repo, out)
}

// emitCollaboratorGroups emits the two collaborator-style groups.
func (c *GithubConnector) emitCollaboratorGroups(
	ctx context.Context, repo GithubRepo, out chan<- interfaces.ExternalGroupOrFailure,
) {
	direct, err := c.client.listCollaborators(ctx, repo.FullName, "direct")
	if err != nil {
		out <- interfaces.NewGroupFailure(entityFailure(repo.FullName, "github: direct collaborators", err))
	} else {
		out <- interfaces.NewGroupResult(&interfaces.ExternalGroup{
			GroupID:      collaboratorsGroupID(repo.ID),
			DisplayName:  repo.FullName + " collaborators",
			MemberEmails: emailsFromUsers(direct),
		})
	}
	outside, err := c.client.listCollaborators(ctx, repo.FullName, "outside")
	if err != nil {
		out <- interfaces.NewGroupFailure(entityFailure(repo.FullName, "github: outside collaborators", err))
		return
	}
	out <- interfaces.NewGroupResult(&interfaces.ExternalGroup{
		GroupID:      outsideCollaboratorsGroupID(repo.ID),
		DisplayName:  repo.FullName + " outside collaborators",
		MemberEmails: emailsFromUsers(outside),
	})
}

// emitTeamGroups walks the repo's teams and emits one group per team.
func (c *GithubConnector) emitTeamGroups(
	ctx context.Context, repo GithubRepo, out chan<- interfaces.ExternalGroupOrFailure,
) {
	teams, err := c.client.listTeams(ctx, repo.FullName)
	if err != nil {
		out <- interfaces.NewGroupFailure(entityFailure(repo.FullName, "github: teams", err))
		return
	}
	for _, t := range teams {
		members, err := c.client.listTeamMembers(ctx, t.ID)
		if err != nil {
			out <- interfaces.NewGroupFailure(entityFailure(repo.FullName, "github: team members "+t.Slug, err))
			continue
		}
		out <- interfaces.NewGroupResult(&interfaces.ExternalGroup{
			GroupID:      teamGroupID(t.Slug),
			DisplayName:  t.Name,
			MemberEmails: emailsFromUsers(members),
		})
	}
}

