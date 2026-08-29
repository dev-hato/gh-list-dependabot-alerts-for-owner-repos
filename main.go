package main

import (
	"context"
	"os"

	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/app"
)

func main() {
	if err := app.Run(context.Background(), os.Args[1:], os.Stdout, app.NewDefaultGithubClient); err != nil {
		if fatalErr := app.Fatal(err, os.Stderr, os.Exit); fatalErr != nil {
			panic(fatalErr)
		}
	}
}

// For more examples of using go-gh, see:
// https://github.com/cli/go-gh/blob/trunk/example_gh_test.go
