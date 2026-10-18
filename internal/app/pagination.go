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

const (
	// MaxPages caps how many pages FetchAllPages follows, so a response can't paginate forever.
	MaxPages = 10000
	// MaxPageBytes caps how much of one response body FetchPage decodes, so a response can't exhaust memory.
	MaxPageBytes = 64 << 20
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

	// A body over MaxPageBytes is cut short, so it fails to decode instead of being read in full.
	if err = json.NewDecoder(io.LimitReader(httpResponse.Body, MaxPageBytes)).Decode(&p.Items); err != nil {
		return p, errors.Wrap(err, "Failed to Decode")
	}

	nextPath, err := LinkHeader(httpResponse.Header.Get("Link")).NextPath(path)
	if err != nil {
		return p, errors.Wrap(err, "Failed to NextPath")
	}

	p.NextPath = nextPath

	return p, nil
}

func (f *PageFetcher[T]) FetchAllPages(ctx context.Context, path string) ([]T, error) {
	var allItems []T

	// The next links come from responses, so guard against a loop back to an earlier page as well as endless new ones.
	visited := make(map[string]struct{})

	// Count fetched pages rather than len(visited), so the cap still holds if the loop check ever misses a cycle.
	for pages := 0; ; pages++ {
		if _, ok := visited[path]; ok {
			return nil, errors.Newf("Pagination loops back to %q", path)
		}

		if pages == MaxPages {
			return nil, errors.Newf("Pagination exceeds %d pages", MaxPages)
		}

		visited[path] = struct{}{}

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
