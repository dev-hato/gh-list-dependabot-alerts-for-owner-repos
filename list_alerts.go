package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cockroachdb/errors"
	"github.com/google/go-github/v89/github"
)

// openAlertsURL builds a "state=open" filtered request path for a Dependabot alerts endpoint.
func openAlertsURL(path string) string {
	u := url.URL{Path: path}
	query := u.Query()
	query.Set("state", "open")
	u.RawQuery = query.Encode()
	return u.String()
}

func listAlertsForOrg(ctx context.Context, client *api.RESTClient, org string) ([]SmallDependabotAlert, error) {
	listOrgAlertsURL := openAlertsURL(fmt.Sprintf("orgs/%s/dependabot/alerts", org))

	alerts, err := fetchAllPages[github.DependabotAlert](ctx, client, listOrgAlertsURL)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to fetchAllPages")
	}

	smallAlerts := make([]SmallDependabotAlert, len(alerts))

	for i, alert := range alerts {
		var repository *SmallRepository

		if alert.Repository != nil {
			repository = &SmallRepository{FullName: alert.Repository.FullName}
		}

		smallAlerts[i] = toSmallDependabotAlert(alert, repository)
	}

	return smallAlerts, nil
}

// fetchAlertsForRepo fetches the open Dependabot alerts for a single "owner/repo".
// It returns (nil, nil) when Dependabot alerts are disabled for the repository.
func fetchAlertsForRepo(ctx context.Context, client *api.RESTClient, ownerRepo string) ([]SmallDependabotAlert, error) {
	listRepoAlertsURL := openAlertsURL(fmt.Sprintf("repos/%s/dependabot/alerts", ownerRepo))

	alerts, err := fetchAllPages[github.DependabotAlert](ctx, client, listRepoAlertsURL)
	if err != nil {
		// The 403 GitHub returns when Dependabot alerts are turned off for the repository.
		var httpErr *api.HTTPError

		if !errors.As(err, &httpErr) {
			return nil, errors.Wrap(err, "Failed to fetchAllPages")
		}

		if httpErr.StatusCode != 403 || !strings.Contains(httpErr.Message, "Dependabot alerts are disabled") {
			return nil, errors.Wrap(err, "Failed to fetchAllPages")
		}

		return nil, nil
	}

	smallAlerts := make([]SmallDependabotAlert, len(alerts))

	for i, alert := range alerts {
		smallAlerts[i] = toSmallDependabotAlert(alert, &SmallRepository{FullName: &ownerRepo})
	}

	return smallAlerts, nil
}

func listAlertsForUser(ctx context.Context, client *api.RESTClient, username string) ([]SmallDependabotAlert, error) {
	repositories, err := fetchAllPages[github.Repository](ctx, client, fmt.Sprintf("users/%s/repos", username))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to fetchAllPages")
	}

	var smallAlerts []SmallDependabotAlert

	for _, repository := range repositories {
		if (repository.Archived != nil && *repository.Archived) || repository.Name == nil {
			continue
		}

		repoSmallAlerts, err := fetchAlertsForRepo(ctx, client, fmt.Sprintf("%s/%s", username, *repository.Name))
		if err != nil {
			return nil, errors.Wrap(err, "Failed to fetchAlertsForRepo")
		}

		smallAlerts = append(smallAlerts, repoSmallAlerts...)
	}

	return smallAlerts, nil
}
