package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cockroachdb/errors"
)

// newDefaultGithubClient builds the production *githubClient:
// a real GitHub REST client paired with the package-level rate limiter.
func newDefaultGithubClient() (*githubClient, error) {
	rest, err := api.DefaultRESTClient()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to api.DefaultRESTClient")
	}

	return &githubClient{rest: rest, limiter: limiter}, nil
}

// errOrgAndUsernameEmpty is returned by run when neither --org nor --username is set.
// It's a package-level sentinel (rather than an inline errors.New) so tests can assert on it with errors.Is.
var errOrgAndUsernameEmpty = errors.New("org and username are both empty")

// run parses args, fetches the requested Dependabot alerts, and writes them as JSON to out.
func run(ctx context.Context, args []string, out io.Writer, newClient func() (*githubClient, error)) error {
	fs := flag.NewFlagSet("gh-list-dependabot-alerts-for-owner-repos", flag.ContinueOnError)
	org := fs.String("org", "", "Target organization name")
	username := fs.String("username", "", "Target username")

	if err := fs.Parse(args); err != nil {
		return errors.Wrap(err, "Failed to fs.Parse")
	}

	if *org == "" && *username == "" {
		return errOrgAndUsernameEmpty
	}

	client, err := newClient()
	if err != nil {
		return errors.Wrap(err, "Failed to newClient")
	}

	var smallAlerts []SmallDependabotAlert

	switch {
	case *org != "":
		smallAlerts, err = listAlertsForOrg(ctx, client, *org)
		if err != nil {
			return errors.Wrap(err, "Failed to listAlertsForOrg")
		}
	default:
		smallAlerts, err = listAlertsForUser(ctx, client, *username)
		if err != nil {
			return errors.Wrap(err, "Failed to listAlertsForUser")
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

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, newDefaultGithubClient); err != nil {
		if fatalErr := fatal(err, os.Stderr, os.Exit); fatalErr != nil {
			panic(fatalErr)
		}
	}
}

// For more examples of using go-gh, see:
// https://github.com/cli/go-gh/blob/trunk/example_gh_test.go
