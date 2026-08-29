package app

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cockroachdb/errors"
	"github.com/google/go-github/v90/github"
	"golang.org/x/sync/errgroup"
)

// OpenAlertsURL builds a "state=open" filtered request path for a Dependabot alerts endpoint.
func OpenAlertsURL(path string) string {
	u := url.URL{Path: path}
	query := u.Query()
	query.Set("state", "open")
	u.RawQuery = query.Encode()
	return u.String()
}

func ListAlertsForOrg(ctx context.Context, client *GithubClient, org string) ([]SmallDependabotAlert, error) {
	listOrgAlertsURL := OpenAlertsURL(fmt.Sprintf("orgs/%s/dependabot/alerts", org))

	alerts, err := FetchAllPages[github.DependabotAlert](ctx, client, listOrgAlertsURL)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to FetchAllPages")
	}

	smallAlerts := make([]SmallDependabotAlert, len(alerts))

	for i, alert := range alerts {
		var repository *SmallRepository

		if alert.Repository != nil {
			repository = &SmallRepository{FullName: alert.Repository.FullName}
		}

		smallAlerts[i] = ToSmallDependabotAlert(alert, repository)
	}

	return smallAlerts, nil
}

// FetchAlertsForRepo fetches the open Dependabot alerts for a single "owner/repo".
// It returns (nil, nil) when Dependabot alerts are disabled for the repository.
func FetchAlertsForRepo(ctx context.Context, client *GithubClient, ownerRepo string) ([]SmallDependabotAlert, error) {
	listRepoAlertsURL := OpenAlertsURL(fmt.Sprintf("repos/%s/dependabot/alerts", ownerRepo))

	alerts, err := FetchAllPages[github.DependabotAlert](ctx, client, listRepoAlertsURL)
	if err != nil {
		// The 403 GitHub returns when Dependabot alerts are turned off for the repository.
		var httpErr *api.HTTPError

		if !errors.As(err, &httpErr) {
			return nil, errors.Wrap(err, "Failed to FetchAllPages")
		}

		if httpErr.StatusCode != 403 || !strings.Contains(httpErr.Message, "Dependabot alerts are disabled") {
			return nil, errors.Wrap(err, "Failed to FetchAllPages")
		}

		return nil, nil
	}

	smallAlerts := make([]SmallDependabotAlert, len(alerts))

	for i, alert := range alerts {
		smallAlerts[i] = ToSmallDependabotAlert(alert, &SmallRepository{FullName: &ownerRepo})
	}

	return smallAlerts, nil
}

// ListAlertsForUser fetches alerts for every non-archived repository of username,
// one repository at a time but in parallel across repositories.
// The shared rate limiter (Limiter.Wait in pagination.go's FetchPage) keeps the combined request rate in check,
// so parallelizing here doesn't burst requests against GitHub.
func ListAlertsForUser(ctx context.Context, client *GithubClient, username string) ([]SmallDependabotAlert, error) {
	repositories, err := FetchAllPages[github.Repository](ctx, client, fmt.Sprintf("users/%s/repos", username))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to FetchAllPages")
	}

	targetRepoNames := make([]string, 0, len(repositories))

	for _, repository := range repositories {
		if (repository.Archived != nil && *repository.Archived) || repository.Name == nil {
			continue
		}

		targetRepoNames = append(targetRepoNames, *repository.Name)
	}

	// Each goroutine writes to its own index,
	// so results keep repository order regardless of which fetch finishes first.
	perRepoAlerts := make([][]SmallDependabotAlert, len(targetRepoNames))

	eg, egCtx := errgroup.WithContext(ctx)

	for i, name := range targetRepoNames {
		eg.Go(func() error {
			repoSmallAlerts, err := FetchAlertsForRepo(egCtx, client, fmt.Sprintf("%s/%s", username, name))
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

	var smallAlerts []SmallDependabotAlert

	for _, repoSmallAlerts := range perRepoAlerts {
		smallAlerts = append(smallAlerts, repoSmallAlerts...)
	}

	return smallAlerts, nil
}
