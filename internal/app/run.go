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

// CLIOptions holds the command-line flag values that NewCLI's flags write into once parsed.
type CLIOptions struct {
	Org  string
	Help bool
}

// CLI bundles the flag set with the CLIOptions its flags populate on Parse.
// They're built together and always used together, so they travel as one value.
type CLI struct {
	FlagSet *flag.FlagSet
	Options *CLIOptions
}

// App holds the wiring Run needs: where the JSON output goes, and how to build the GitHub client.
// Both are fields so tests can capture the output and inject a fake client.
type App struct {
	Out       io.Writer
	NewClient func() (*GithubClient, error)
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

// NewCLI builds the CLI's flag set together with the CLIOptions its flags populate on Parse.
// Both Run and its tests build the flag set through here
// so the registered flags (and thus PrintUsage's output) never drift between the two.
func NewCLI() *CLI {
	opts := &CLIOptions{}
	fs := flag.NewFlagSet("list-dependabot-alerts-for-owner-repos", flag.ContinueOnError)
	fs.StringVar(&opts.Org, "org", "", "Target organization name")
	fs.BoolVar(&opts.Help, "help", false, "Show this help and exit")
	fs.BoolVar(&opts.Help, "h", false, "Show this help and exit")
	return &CLI{FlagSet: fs, Options: opts}
}

// PrintUsage writes the --help/-h usage text and flag defaults to out.
func (c *CLI) PrintUsage(out io.Writer) error {
	if _, err := io.WriteString(out, fmt.Sprintf(`Usage: gh %s [options]

A GitHub CLI extension that lists Dependabot alerts (vulnerability alerts from Dependabot).
It covers every repository owned by an organization or a user.

Options:
`, c.FlagSet.Name())); err != nil {
		return errors.Wrap(err, "Failed to io.WriteString")
	}

	c.FlagSet.SetOutput(out)
	c.FlagSet.PrintDefaults()
	return nil
}

// Run parses args, fetches the requested Dependabot alerts, and writes them as JSON to a.Out.
func (a *App) Run(ctx context.Context, args []string) error {
	cli := NewCLI()

	if err := cli.FlagSet.Parse(args); err != nil {
		return errors.Wrap(err, "Failed to FlagSet.Parse")
	}

	if cli.Options.Help {
		return errors.Wrap(cli.PrintUsage(a.Out), "Failed to PrintUsage")
	}

	client, err := a.NewClient()
	if err != nil {
		return errors.Wrap(err, "Failed to newClient")
	}

	smallAlerts, err := client.ListAlerts(ctx, cli.Options.Org)
	if err != nil {
		return errors.Wrap(err, "Failed to ListAlerts")
	}

	smallAlertsJSONBytes, err := json.MarshalIndent(smallAlerts, "", "\t")
	if err != nil {
		return errors.Wrap(err, "Failed to json.Marshal")
	}

	if _, err := fmt.Fprintln(a.Out, string(smallAlertsJSONBytes)); err != nil {
		return errors.Wrap(err, "Failed to fmt.Fprintln")
	}

	return nil
}
