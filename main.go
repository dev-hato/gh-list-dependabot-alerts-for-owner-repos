package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cockroachdb/errors"
)

func main() {
	org := flag.String("org", "", "Target organization name")
	username := flag.String("username", "", "Target username")
	flag.Parse()

	client, err := api.DefaultRESTClient()
	if err != nil {
		fatal(errors.Wrap(err, "Failed to api.DefaultRESTClient"))
	}

	ctx := context.Background()
	var smallAlerts []SmallDependabotAlert

	if *org != "" {
		smallAlerts, err = listAlertsForOrg(ctx, client, *org)
		if err != nil {
			fatal(errors.Wrap(err, "Failed to listAlertsForOrg"))
		}
	} else if *username != "" {
		smallAlerts, err = listAlertsForUser(ctx, client, *username)
		if err != nil {
			fatal(errors.Wrap(err, "Failed to listAlertsForUser"))
		}
	} else {
		fatal(errors.New("org and username are both empty"))
	}

	smallAlertsJsonBytes, err := json.MarshalIndent(smallAlerts, "", "\t")
	if err != nil {
		fatal(errors.Wrap(err, "Failed to json.Marshal"))
	}

	fmt.Println(string(smallAlertsJsonBytes))
}

// For more examples of using go-gh, see:
// https://github.com/cli/go-gh/blob/trunk/example_gh_test.go
