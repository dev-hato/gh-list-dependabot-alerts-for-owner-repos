package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cockroachdb/errors"
)

// NewDefaultGithubClient builds the production *GithubClient:
// a real GitHub REST client paired with the package-level rate limiter.
func NewDefaultGithubClient() (*GithubClient, error) {
	rest, err := api.DefaultRESTClient()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to DefaultRESTClient")
	}

	return &GithubClient{Rest: rest, Limiter: limiter}, nil
}

// Run parses args, fetches the requested Dependabot alerts, and writes them as JSON to out.
func Run(ctx context.Context, args []string, out io.Writer, newClient func() (*GithubClient, error)) error {
	var help bool

	fs := flag.NewFlagSet("gh-list-dependabot-alerts-for-owner-repos", flag.ContinueOnError)
	org := fs.String("org", "", "Target organization name")
	fs.BoolVar(&help, "help", false, "Show this help and exit")
	fs.BoolVar(&help, "h", false, "Show this help and exit")

	if err := fs.Parse(args); err != nil {
		return errors.Wrap(err, "Failed to fs.Parse")
	}

	if help {
		if _, err := io.WriteString(out, `Usage: gh list-dependabot-alerts-for-owner-repos [options]

A GitHub CLI extension that lists Dependabot alerts (vulnerability alerts from Dependabot).
It covers every repository owned by an organization or a user.

Options:
`); err != nil {
			return errors.Wrap(err, "Failed to io.WriteString")
		}

		fs.SetOutput(out)
		fs.PrintDefaults()
		return nil
	}

	client, err := newClient()
	if err != nil {
		return errors.Wrap(err, "Failed to newClient")
	}

	var smallAlerts []SmallDependabotAlert

	switch {
	case *org != "":
		smallAlerts, err = ListAlertsForOrg(ctx, client, *org)
		if err != nil {
			return errors.Wrap(err, "Failed to ListAlertsForOrg")
		}
	default:
		smallAlerts, err = ListAlertsForUser(ctx, client)
		if err != nil {
			return errors.Wrap(err, "Failed to ListAlertsForUser")
		}
	}

	smallAlertsJSONBytes, err := json.MarshalIndent(smallAlerts, "", "\t")
	if err != nil {
		return errors.Wrap(err, "Failed to json.Marshal")
	}

	if _, err := fmt.Fprintln(out, string(smallAlertsJSONBytes)); err != nil {
		return errors.Wrap(err, "Failed to fmt.Fprintln")
	}

	return nil
}
