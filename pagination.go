package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cockroachdb/errors"
)

type Page[T any] struct {
	items    []T
	nextPath string
}

// waitBeforeCall sleeps for waiter's next backoff duration, logging it and honoring ctx cancellation while waiting.
func waitBeforeCall(ctx context.Context) error {
	wait, err := waiter.Wait()
	if err != nil {
		return errors.Wrap(err, "Failed to waiter.Wait")
	}

	if wait != time.Duration(0) {
		if _, err := fmt.Fprintf(os.Stderr, "Wait %s\n", wait); err != nil {
			return errors.Wrap(err, "Failed to fmt.Fprintf")
		}
	}

	select {
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "Context done while backing off")
	case <-time.After(wait):
	}

	return nil
}

func fetchPage[T any](ctx context.Context, client *api.RESTClient, path string) (p Page[T], err error) {
	p.nextPath = path

	if err = waitBeforeCall(ctx); err != nil {
		return p, errors.Wrap(err, "Failed to waitBeforeCall")
	}

	if _, err = fmt.Fprintf(os.Stderr, "Call %s\n", path); err != nil {
		return p, errors.Wrap(err, "Failed to fmt.Fprintf")
	}

	httpResponse, err := client.RequestWithContext(ctx, http.MethodGet, path, nil)
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

func fetchAllPages[T any](ctx context.Context, client *api.RESTClient, path string) ([]T, error) {
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
