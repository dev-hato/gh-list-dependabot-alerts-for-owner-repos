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

// CLIOptions holds the command-line flag values that NewFlagSet's flags write into once parsed.
type CLIOptions struct {
	Org  string
	Help bool
}

// NewDefaultGithubClient builds the production *GithubClient:
// a real GitHub REST client paired with the package-level rate limiter.
func NewDefaultGithubClient() (*GithubClient, error) {
	rest, err := api.DefaultRESTClient()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to DefaultRESTClient")
	}

	return &GithubClient{Rest: rest, Limiter: limiter}, nil
}

// NewFlagSet builds the CLI's flag set, returning it together with the CLIOptions its flags populate on Parse.
// Both Run and its tests build the flag set through here
// so the registered flags (and thus PrintUsage's output) never drift between the two.
func NewFlagSet() (*flag.FlagSet, *CLIOptions) {
	opts := &CLIOptions{}
	fs := flag.NewFlagSet("gh-list-dependabot-alerts-for-owner-repos", flag.ContinueOnError)
	fs.StringVar(&opts.Org, "org", "", "Target organization name")
	fs.BoolVar(&opts.Help, "help", false, "Show this help and exit")
	fs.BoolVar(&opts.Help, "h", false, "Show this help and exit")
	return fs, opts
}

// Run parses args, fetches the requested Dependabot alerts, and writes them as JSON to out.
func Run(ctx context.Context, args []string, out io.Writer, newClient func() (*GithubClient, error)) error {
	fs, opts := NewFlagSet()

	if err := fs.Parse(args); err != nil {
		return errors.Wrap(err, "Failed to fs.Parse")
	}

	if opts.Help {
		return errors.Wrap(PrintUsage(out, fs), "Failed to PrintUsage")
	}

	client, err := newClient()
	if err != nil {
		return errors.Wrap(err, "Failed to newClient")
	}

	smallAlerts, err := ListAlerts(ctx, client, opts.Org)
	if err != nil {
		return errors.Wrap(err, "Failed to ListAlerts")
	}

	smallAlertsJSONBytes, err := json.MarshalIndent(smallAlerts, "", "\t")
	if err != nil {
		return errors.Wrap(err, "Failed to json.Marshal")
	}

	_, err = fmt.Fprintln(out, string(smallAlertsJSONBytes))
	return errors.Wrap(err, "Failed to fmt.Fprintln")
}

// PrintUsage writes the --help/-h usage text and flag defaults to out.
func PrintUsage(out io.Writer, fs *flag.FlagSet) error {
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

// ListAlerts picks the org path when org is non-empty, and the authenticated-user path otherwise.
func ListAlerts(ctx context.Context, client *GithubClient, org string) ([]SmallDependabotAlert, error) {
	if org != "" {
		smallAlerts, err := ListAlertsForOrg(ctx, client, org)
		return smallAlerts, errors.Wrap(err, "Failed to ListAlertsForOrg")
	}

	smallAlerts, err := ListAlertsForUser(ctx, client)
	return smallAlerts, errors.Wrap(err, "Failed to ListAlertsForUser")
}
