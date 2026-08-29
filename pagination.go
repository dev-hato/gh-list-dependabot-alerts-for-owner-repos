package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cockroachdb/errors"
	"golang.org/x/time/rate"
)

type Page[T any] struct {
	items    []T
	nextPath string
}

// githubClient bundles the REST client with the rate limiter that throttles it.
// Every fetch needs both together, so they travel as one parameter instead of two.
type githubClient struct {
	rest    *api.RESTClient
	limiter *rate.Limiter
}

func fetchPage[T any](ctx context.Context, client *githubClient, path string) (p Page[T], err error) {
	p.nextPath = path

	if err = client.limiter.Wait(ctx); err != nil {
		return p, errors.Wrap(err, "Failed to limiter.Wait")
	}

	if _, err = fmt.Fprintf(os.Stderr, "Call %s\n", path); err != nil {
		return p, errors.Wrap(err, "Failed to fmt.Fprintf")
	}

	httpResponse, err := client.rest.RequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return p, errors.Wrap(err, "Failed to client.RequestWithContext")
	}
	defer func(body io.ReadCloser) {
		if closeErr := body.Close(); closeErr != nil {
			err = errors.Join(err, errors.Wrap(closeErr, "Failed to Close"))
		}
	}(httpResponse.Body)

	if err := json.NewDecoder(httpResponse.Body).Decode(&p.items); err != nil {
		return p, errors.Wrap(err, "Failed to Decode")
	}

	p.nextPath = nextPathFromLink(httpResponse.Header.Get("Link"), path)

	return p, nil
}

func fetchAllPages[T any](ctx context.Context, client *githubClient, path string) ([]T, error) {
	var allItems []T

	for {
		p, err := fetchPage[T](ctx, client, path)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to fetchPage")
		}

		allItems = append(allItems, p.items...)

		if path == p.nextPath {
			return allItems, nil
		}

		path = p.nextPath
	}
}
