package app_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/app"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/time/rate"
)

var errCloseBoom = errors.New("close boom")

// paginationGuardTimeout bounds the pagination-guard tests, which would otherwise loop forever if a guard regressed.
const paginationGuardTimeout = 30 * time.Second

type testItem struct {
	Number int `json:"number"`
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write boom")
}

type closeErrReadCloser struct {
	io.Reader
	closeErr error
}

func (c closeErrReadCloser) Close() error {
	return c.closeErr
}

// nextPage2Handler returns an http.HandlerFunc that responds with body and a Link header pointing to page 2,
// as an application/json response.
func nextPage2Handler(t *testing.T, body string) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<https://api.github.com/orgs/foo/dependabot/alerts?page=2>; rel="next"`)

		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}
}

// twoPageHandler returns an http.HandlerFunc serving page 1 (linking to page 2) and page 2, counting requests in calls.
// page2Link is page 2's Link header; empty means page 2 is the last page.
func twoPageHandler(t *testing.T, calls *int, page2Link string) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		*calls++

		if r.URL.Query().Get("page") == "2" {
			if page2Link != "" {
				w.Header().Set("Link", page2Link)
			}

			jsonHandler(t, http.StatusOK, `[{"number":2}]`)(w, r)
			return
		}

		nextPage2Handler(t, `[{"number":1}]`)(w, r)
	}
}

// fetchAllPagesWithinGuardTimeout runs FetchAllPages against client under paginationGuardTimeout.
// The pagination-guard tests use it because, without the guard, FetchAllPages would paginate forever,
// so the deadline makes them fail fast instead of hanging.
func fetchAllPagesWithinGuardTimeout(t *testing.T, client *app.GithubClient) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), paginationGuardTimeout)
	defer cancel()

	_, err := app.NewPageFetcher[testItem](client).FetchAllPages(ctx, "orgs/foo/dependabot/alerts")
	return err
}

func TestFetchPage(t *testing.T) {
	tests := map[string]struct {
		newClient       func(t *testing.T) *app.GithubClient
		wantErrContains string // empty means no error expected
		wantErrIs       error  // optional: also checked with errors.Is
		check           func(t *testing.T, p app.Page[testItem])
	}{
		"success without a next link": {
			newClient: func(t *testing.T) *app.GithubClient {
				return newTestGithubClient(t, jsonHandler(t, http.StatusOK, `[{"number":1},{"number":2}]`))
			},
			check: func(t *testing.T, p app.Page[testItem]) {
				if diff := cmp.Diff([]testItem{{Number: 1}, {Number: 2}}, p.Items); diff != "" {
					t.Errorf("items mismatch (-want +got):\n%s", diff)
				}

				if p.NextPath != "orgs/foo/dependabot/alerts" {
					t.Errorf("nextPath = %q, want unchanged path", p.NextPath)
				}
			},
		},
		"success with a next link": {
			newClient: func(t *testing.T) *app.GithubClient {
				return newTestGithubClient(t, nextPage2Handler(t, `[{"number":1}]`))
			},
			check: func(t *testing.T, p app.Page[testItem]) {
				if p.NextPath != "orgs/foo/dependabot/alerts?page=2" {
					t.Errorf("nextPath = %q, want the next page path", p.NextPath)
				}
			},
		},
		"HTTP error is wrapped": {
			newClient: func(t *testing.T) *app.GithubClient {
				return newTestGithubClient(t, jsonHandler(t, http.StatusInternalServerError, `{"message":"boom"}`))
			},
			wantErrContains: "Failed to client.RequestWithContext",
		},
		"malformed JSON body is wrapped as a decode error": {
			newClient: func(t *testing.T) *app.GithubClient {
				return newTestGithubClient(t, jsonHandler(t, http.StatusOK, `not json`))
			},
			wantErrContains: "Failed to Decode",
		},
		"body close error surfaces even on an otherwise successful decode": {
			newClient: func(t *testing.T) *app.GithubClient {
				return &app.GithubClient{
					Rest: newRESTClientWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": []string{"application/json"}},
							Body:       closeErrReadCloser{Reader: strings.NewReader(`[]`), closeErr: errCloseBoom},
							Request:    req,
						}, nil
					})),
					Limiter: noWaitLimiter(),
				}
			},
			wantErrIs: errCloseBoom,
		},
		"limiter.Wait failure is wrapped": {
			newClient: func(t *testing.T) *app.GithubClient {
				return &app.GithubClient{
					Rest: newTestRESTClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
						t.Error("the server should not be called when waiting fails")
					})),
					// A zero-burst limiter can never admit a single request, so Wait fails immediately.
					Limiter: rate.NewLimiter(0, 0),
				}
			},
			wantErrContains: "Failed to Limiter.Wait",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := app.NewPageFetcher[testItem](tt.newClient(t)).FetchPage(context.Background(), "orgs/foo/dependabot/alerts")

			if tt.wantErrContains == "" && tt.wantErrIs == nil {
				if err != nil {
					t.Fatalf("fetchPage() error = %v, want nil", err)
				}

				tt.check(t, p)
				return
			}

			if tt.wantErrContains != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrContains)) {
				t.Errorf("fetchPage() error = %v, want it to mention %q", err, tt.wantErrContains)
			}

			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Errorf("fetchPage() error = %v, want it to wrap %v", err, tt.wantErrIs)
			}
		})
	}
}

func TestFetchAllPages(t *testing.T) {
	t.Run("aggregates items across pages", func(t *testing.T) {
		var calls int

		client := newTestGithubClient(t, twoPageHandler(t, &calls, ""))

		got, err := app.NewPageFetcher[testItem](client).FetchAllPages(context.Background(), "orgs/foo/dependabot/alerts")
		if err != nil {
			t.Fatalf("fetchAllPages() error = %v, want nil", err)
		}

		if diff := cmp.Diff([]testItem{{Number: 1}, {Number: 2}}, got); diff != "" {
			t.Errorf("fetchAllPages() mismatch (-want +got):\n%s", diff)
		}

		if calls != 2 {
			t.Errorf("server was called %d times, want 2", calls)
		}
	})

	t.Run("propagates a fetchPage error", func(t *testing.T) {
		client := newTestGithubClient(t, jsonHandler(t, http.StatusInternalServerError, `{"message":"boom"}`))

		_, err := app.NewPageFetcher[testItem](client).FetchAllPages(context.Background(), "orgs/foo/dependabot/alerts")
		if err == nil || !strings.Contains(err.Error(), "Failed to FetchPage") {
			t.Errorf("fetchAllPages() error = %v, want it to mention fetchPage", err)
		}
	})

	t.Run("a next link back to an earlier page is an error", func(t *testing.T) {
		var calls int

		client := newTestGithubClient(
			t,
			twoPageHandler(t, &calls, `<https://api.github.com/orgs/foo/dependabot/alerts>; rel="next"`),
		)

		err := fetchAllPagesWithinGuardTimeout(t, client)
		if err == nil || !strings.Contains(err.Error(), "Pagination loops back") {
			t.Errorf("fetchAllPages() error = %v, want it to mention the loop", err)
		}

		if calls != 2 {
			t.Errorf("server was called %d times, want 2", calls)
		}
	})

	t.Run("endless new next links stop at MaxPages", func(t *testing.T) {
		var calls int

		client := newTestGithubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set(
				"Link",
				fmt.Sprintf(`<https://api.github.com/orgs/foo/dependabot/alerts?page=%d>; rel="next"`, calls+1),
			)
			jsonHandler(t, http.StatusOK, `[]`)(w, r)
		}))

		err := fetchAllPagesWithinGuardTimeout(t, client)
		if err == nil || !strings.Contains(err.Error(), "Pagination exceeds") {
			t.Errorf("fetchAllPages() error = %v, want it to mention the page cap", err)
		}

		if calls != app.MaxPages {
			t.Errorf("server was called %d times, want %d", calls, app.MaxPages)
		}
	})
}

func TestFetchPageRejectsOversizedBody(t *testing.T) {
	client := newTestGithubClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if _, err := w.Write([]byte("[")); err != nil {
			t.Error(err)
			return
		}

		chunk := bytes.Repeat([]byte(`{"number":1},`), 1<<16)

		for written := 0; written <= app.MaxPageBytes; written += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}

		if _, err := w.Write([]byte(`{"number":1}]`)); err != nil {
			return
		}
	}))

	_, err := app.NewPageFetcher[testItem](client).FetchPage(t.Context(), "orgs/foo/dependabot/alerts")
	if err == nil || !strings.Contains(err.Error(), "Failed to Decode") {
		t.Errorf("FetchPage() error = %v, want it to mention Decode", err)
	}
}
