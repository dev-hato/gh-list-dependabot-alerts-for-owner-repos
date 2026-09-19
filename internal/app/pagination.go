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
// Every fetch needs both together, so they travel as one value instead of two.
type GithubClient struct {
	Rest    *api.RESTClient
	Limiter *rate.Limiter
}

// PageFetcher fetches paginated GitHub API responses whose items decode into T.
// The item type rides on the receiver because a Go method can't declare its own type parameter,
// which is what lets FetchPage and FetchAllPages stay single-argument methods.
type PageFetcher[T any] struct {
	Client *GithubClient
}

// NewPageFetcher pairs client with the item type T its pages decode into.
func NewPageFetcher[T any](client *GithubClient) *PageFetcher[T] {
	return &PageFetcher[T]{Client: client}
}

func (f *PageFetcher[T]) FetchPage(ctx context.Context, path string) (p Page[T], err error) {
	p.NextPath = path

	if err = f.Client.Limiter.Wait(ctx); err != nil {
		return p, errors.Wrap(err, "Failed to Limiter.Wait")
	}

	if _, err = fmt.Fprintf(os.Stderr, "Call %s\n", path); err != nil {
		return p, errors.Wrap(err, "Failed to fmt.Fprintf")
	}

	httpResponse, err := f.Client.Rest.RequestWithContext(ctx, http.MethodGet, path, nil)
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

	p.NextPath = LinkHeader(httpResponse.Header.Get("Link")).NextPath(path)

	return p, nil
}

func (f *PageFetcher[T]) FetchAllPages(ctx context.Context, path string) ([]T, error) {
	var allItems []T

	for {
		p, err := f.FetchPage(ctx, path)
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
