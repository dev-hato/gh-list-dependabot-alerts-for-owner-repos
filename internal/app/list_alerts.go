package app

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cockroachdb/errors"
	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/slice"
	"github.com/google/go-github/v92/github"
	"golang.org/x/sync/errgroup"
)

// AlertsScope selects which Dependabot alerts endpoint OpenAlertsURL targets:
// an organization's alerts across all its repos, or a single repository's alerts.
// It also supplies the leading path segment for that endpoint.
// Its only valid values are OrgScope and RepoScope.
type AlertsScope string

const (
	// OrgScope targets "orgs/{org}/dependabot/alerts".
	OrgScope AlertsScope = "orgs"
	// RepoScope targets "repos/{owner}/{repo}/dependabot/alerts".
	RepoScope AlertsScope = "repos"
)

// OpenAlertsURL builds a "state=open" filtered request path for this scope's Dependabot alerts endpoint.
// target is the "{org}" or "{owner}/{repo}" segment that follows the scope.
func (s AlertsScope) OpenAlertsURL(target string) string {
	u := url.URL{Path: fmt.Sprintf("%s/%s/dependabot/alerts", s, target)}
	query := u.Query()
	query.Set("state", "open")
	u.RawQuery = query.Encode()
	return u.String()
}

// ListAlerts picks the org path when org is non-empty, and the authenticated-user path otherwise.
func (c *GithubClient) ListAlerts(ctx context.Context, org string) ([]SmallDependabotAlert, error) {
	if org != "" {
		smallAlerts, err := c.ListAlertsForOrg(ctx, org)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to ListAlertsForOrg")
		}

		return smallAlerts, nil
	}

	smallAlerts, err := c.ListAlertsForUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to ListAlertsForUser")
	}

	return smallAlerts, nil
}

func (c *GithubClient) ListAlertsForOrg(ctx context.Context, org string) ([]SmallDependabotAlert, error) {
	listOrgAlertsURL := OrgScope.OpenAlertsURL(org)

	alerts, err := NewPageFetcher[DependabotAlert](c).FetchAllPages(ctx, listOrgAlertsURL)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to FetchAllPages")
	}

	return slice.Map(alerts, func(alert DependabotAlert) SmallDependabotAlert {
		return alert.ToSmall(alert.ToSmallRepository())
	}), nil
}

// IsDependabotAlertsDisabled reports whether err is the 403 GitHub returns
// when Dependabot alerts are turned off for the repository.
func IsDependabotAlertsDisabled(err error) bool {
	var httpErr *api.HTTPError

	if !errors.As(err, &httpErr) {
		return false
	}

	return httpErr.StatusCode == 403 && strings.Contains(httpErr.Message, "Dependabot alerts are disabled")
}

// FetchAlertsForRepo fetches the open Dependabot alerts for a single "owner/repo".
// It returns (nil, nil) when Dependabot alerts are disabled for the repository.
func (c *GithubClient) FetchAlertsForRepo(ctx context.Context, ownerRepo string) ([]SmallDependabotAlert, error) {
	listRepoAlertsURL := RepoScope.OpenAlertsURL(ownerRepo)

	alerts, err := NewPageFetcher[DependabotAlert](c).FetchAllPages(ctx, listRepoAlertsURL)
	if IsDependabotAlertsDisabled(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "Failed to FetchAllPages")
	}

	repo := &SmallRepository{FullName: &ownerRepo}
	return slice.Map(alerts, func(alert DependabotAlert) SmallDependabotAlert {
		return alert.ToSmall(repo)
	}), nil
}

// ListAlertsForUser fetches alerts for every non-archived repository owned by the authenticated user,
// one repository at a time but in parallel across repositories.
// The shared rate limiter (Limiter.Wait in pagination.go's FetchPage) keeps the combined request rate in check,
// so parallelizing here doesn't burst requests against GitHub.
func (c *GithubClient) ListAlertsForUser(ctx context.Context) ([]SmallDependabotAlert, error) {
	// GET /user/repos defaults to affiliation=owner,collaborator,organization_member.
	// type=owner narrows that to repos the user actually owns,
	// matching this extension's "owner repos" scope
	// instead of also pulling in repos the user merely collaborates on or reaches via an org membership.
	u := url.URL{Path: "user/repos"}
	query := u.Query()
	query.Set("type", "owner")
	u.RawQuery = query.Encode()

	repositories, err := NewPageFetcher[github.Repository](c).FetchAllPages(ctx, u.String())
	if err != nil {
		return nil, errors.Wrap(err, "Failed to FetchAllPages")
	}

	targetRepoNames := make([]string, 0, len(repositories))

	for _, repository := range repositories {
		if (repository.Archived != nil && *repository.Archived) || repository.FullName == nil {
			continue
		}

		targetRepoNames = append(targetRepoNames, *repository.FullName)
	}

	// Each goroutine writes to its own index,
	// so results keep repository order regardless of which fetch finishes first.
	perRepoAlerts := make([][]SmallDependabotAlert, len(targetRepoNames))

	eg, egCtx := errgroup.WithContext(ctx)

	for i, fullName := range targetRepoNames {
		eg.Go(func() error {
			repoSmallAlerts, err := c.FetchAlertsForRepo(egCtx, fullName)
			if err != nil {
				return errors.Wrap(err, "Failed to FetchAlertsForRepo")
			}

			perRepoAlerts[i] = repoSmallAlerts
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, errors.Wrap(err, "Failed to Wait")
	}

	return slices.Concat(perRepoAlerts...), nil
}
