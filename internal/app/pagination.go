package app

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
	Items    []T
	NextPath string
}

// GithubClient bundles the REST client with the rate limiter that throttles it.
// Every fetch needs both together, so they travel as one parameter instead of two.
type GithubClient struct {
	Rest    *api.RESTClient
	Limiter *rate.Limiter
}

func FetchPage[T any](ctx context.Context, client *GithubClient, path string) (p Page[T], err error) {
	p.NextPath = path

	if err = client.Limiter.Wait(ctx); err != nil {
		return p, errors.Wrap(err, "Failed to Limiter.Wait")
	}

	if _, err = fmt.Fprintf(os.Stderr, "Call %s\n", path); err != nil {
		return p, errors.Wrap(err, "Failed to fmt.Fprintf")
	}

	httpResponse, err := client.Rest.RequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return p, errors.Wrap(err, "Failed to client.RequestWithContext")
	}
	defer func(body io.ReadCloser) {
		if closeErr := body.Close(); closeErr != nil {
			err = errors.Join(err, errors.Wrap(closeErr, "Failed to Close"))
		}
	}(httpResponse.Body)

	if err := json.NewDecoder(httpResponse.Body).Decode(&p.Items); err != nil {
		return p, errors.Wrap(err, "Failed to Decode")
	}

	p.NextPath = NextPathFromLink(httpResponse.Header.Get("Link"), path)

	return p, nil
}

func FetchAllPages[T any](ctx context.Context, client *GithubClient, path string) ([]T, error) {
	var allItems []T

	for {
		p, err := FetchPage[T](ctx, client, path)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to FetchPage")
		}

		allItems = append(allItems, p.Items...)

		if path == p.NextPath {
			return allItems, nil
		}

		path = p.NextPath
	}
}
