package main

import (
	"context"
	"os"

	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/app"
)

func main() {
	a := &app.App{Out: os.Stdout, NewClient: app.NewDefaultGithubClient}

	if err := a.Run(context.Background(), os.Args[1:]); err != nil {
		reporter := &app.FatalReporter{Out: os.Stderr, Exit: os.Exit}
		if reportErr := reporter.Report(err); reportErr != nil {
			panic(reportErr)
		}
	}
}

// For more examples of using go-gh, see:
// https://github.com/cli/go-gh/blob/trunk/example_gh_test.go
