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

// Page is one decoded page of items.
// NextPath is the next page's path, or "" when the response linked no next page.
type Page[T any] struct {
	Items    []T
	NextPath string
}

// GithubClient bundles the REST client with the rate limiter that throttles it
// and the budget that caps how much of its responses get decoded.
// Every fetch needs all three together, so they travel as one value.
type GithubClient struct {
	Rest    *api.RESTClient
	Limiter *rate.Limiter
	Budget  *Budget
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

	// A body over MaxPageBytes, or over what's left of the shared budget, is cut short,
	// so it fails to decode instead of being read in full.
	body := &io.LimitedReader{R: BudgetReader{R: httpResponse.Body, Budget: f.Client.Budget}, N: MaxPageBytes}

	if p.Items, err = f.decodeItems(json.NewDecoder(body)); err != nil {
		if body.N == 0 {
			return p, errors.Wrapf(err, "Failed to decodeItems within the %d-byte limit", MaxPageBytes)
		}

		return p, errors.Wrap(err, "Failed to decodeItems")
	}

	nextPath, err := LinkHeader(httpResponse.Header.Get("Link")).NextPath()
	if err != nil {
		return p, errors.Wrap(err, "Failed to NextPath")
	}

	p.NextPath = nextPath

	return p, nil
}

// decodeItems decodes a JSON array one item at a time,
// spending each item from the budget before decoding it,
// so items past the limit are never decoded or kept.
func (f *PageFetcher[T]) decodeItems(dec *json.Decoder) ([]T, error) {
	start, err := dec.Token()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to Token")
	}

	if start != json.Delim('[') {
		// The token comes from the response, so keep its value out of the error, which ends up on stderr.
		return nil, errors.New("Response body is not a JSON array")
	}

	var items []T

	for dec.More() {
		if !f.Client.Budget.SpendItems(1) {
			return nil, errors.New("Pagination exceeds the item limit")
		}

		var item T
		if err = dec.Decode(&item); err != nil {
			return nil, errors.Wrap(err, "Failed to Decode")
		}

		items = append(items, item)
	}

	if _, err = dec.Token(); err != nil {
		return nil, errors.Wrap(err, "Failed to Token")
	}

	return items, nil
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

		if p.NextPath == "" {
			return allItems, nil
		}

		path = p.NextPath
	}
}
