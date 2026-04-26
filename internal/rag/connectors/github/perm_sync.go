package github

import (
	"context"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// mapVisibility produces the same group IDs SyncExternalGroups emits
// for the same repo — both sides must agree byte-for-byte or the
// per-doc ACL won't match the published group catalog.
func mapVisibility(repo GithubRepo) *interfaces.ExternalAccess {
	if isPublic(repo) {
		return &interfaces.ExternalAccess{IsPublic: true}
	}
	if isInternal(repo) {
		return &interfaces.ExternalAccess{
			ExternalUserGroupIDs: []string{orgGroupID(repo.Owner.ID)},
		}
	}
	return &interfaces.ExternalAccess{
		ExternalUserGroupIDs: []string{
			collaboratorsGroupID(repo.ID),
			outsideCollaboratorsGroupID(repo.ID),
		},
	}
}

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

// A partial failure (one team's members fail) does not kill the whole
// sync — each helper wraps its own error path.
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
	c.emitCollaboratorGroups(ctx, repo, out)
	c.emitTeamGroups(ctx, repo, out)
}

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

