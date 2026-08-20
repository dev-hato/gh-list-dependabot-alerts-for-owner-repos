package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/time/rate"
)

var errCloseBoom = errors.New("close boom")

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

// fetchPageTestCase is the table shape for TestFetchPage.
// Named (rather than anonymous) so checkFetchPageResult can take it as a parameter,
// keeping the assertion logic out of TestFetchPage's own cyclomatic complexity.
type fetchPageTestCase struct {
	newClient       func(t *testing.T) *githubClient
	wantErrContains string // empty means no error expected
	wantErrIs       error  // optional: also checked with errors.Is
	check           func(t *testing.T, p Page[testItem])
}

func checkFetchPageResult(t *testing.T, tt fetchPageTestCase, p Page[testItem], err error) {
	t.Helper()

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
}

func TestFetchPage(t *testing.T) {
	tests := map[string]fetchPageTestCase{
		"success without a next link": {
			newClient: func(t *testing.T) *githubClient {
				return newTestGithubClient(t, jsonHandler(t, http.StatusOK, `[{"number":1},{"number":2}]`))
			},
			check: func(t *testing.T, p Page[testItem]) {
				if len(p.items) != 2 || p.items[0].Number != 1 || p.items[1].Number != 2 {
					t.Errorf("items = %+v, want [{1} {2}]", p.items)
				}

				if p.nextPath != "orgs/foo/dependabot/alerts" {
					t.Errorf("nextPath = %q, want unchanged path", p.nextPath)
				}
			},
		},
		"success with a next link": {
			newClient: func(t *testing.T) *githubClient {
				return newTestGithubClient(t, nextPage2Handler(t, `[{"number":1}]`))
			},
			check: func(t *testing.T, p Page[testItem]) {
				if p.nextPath != "orgs/foo/dependabot/alerts?page=2" {
					t.Errorf("nextPath = %q, want the next page path", p.nextPath)
				}
			},
		},
		"HTTP error is wrapped": {
			newClient: func(t *testing.T) *githubClient {
				return newTestGithubClient(t, jsonHandler(t, http.StatusInternalServerError, `{"message":"boom"}`))
			},
			wantErrContains: "Failed to client.RequestWithContext",
		},
		"malformed JSON body is wrapped as a decode error": {
			newClient: func(t *testing.T) *githubClient {
				return newTestGithubClient(t, jsonHandler(t, http.StatusOK, `not json`))
			},
			wantErrContains: "Failed to Decode",
		},
		"body close error surfaces even on an otherwise successful decode": {
			newClient: func(t *testing.T) *githubClient {
				return &githubClient{
					rest: newRESTClientWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": []string{"application/json"}},
							Body:       closeErrReadCloser{Reader: strings.NewReader(`[]`), closeErr: errCloseBoom},
							Request:    req,
						}, nil
					})),
					limiter: noWaitLimiter(),
				}
			},
			wantErrIs: errCloseBoom,
		},
		"limiter.Wait failure is wrapped": {
			newClient: func(t *testing.T) *githubClient {
				return &githubClient{
					rest: newTestRESTClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
						t.Error("the server should not be called when waiting fails")
					})),
					// A zero-burst limiter can never admit a single request, so Wait fails immediately.
					limiter: rate.NewLimiter(0, 0),
				}
			},
			wantErrContains: "Failed to limiter.Wait",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := fetchPage[testItem](context.Background(), tt.newClient(t), "orgs/foo/dependabot/alerts")
			checkFetchPageResult(t, tt, p, err)
		})
	}
}

func TestFetchAllPages(t *testing.T) {
	t.Run("aggregates items across pages", func(t *testing.T) {
		var calls int

		client := newTestGithubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++

			if r.URL.Query().Get("page") == "2" {
				jsonHandler(t, http.StatusOK, `[{"number":2}]`)(w, r)
				return
			}

			nextPage2Handler(t, `[{"number":1}]`)(w, r)
		}))

		items, err := fetchAllPages[testItem](context.Background(), client, "orgs/foo/dependabot/alerts")
		if err != nil {
			t.Fatalf("fetchAllPages() error = %v, want nil", err)
		}

		if len(items) != 2 || items[0].Number != 1 || items[1].Number != 2 {
			t.Errorf("items = %+v, want [{1} {2}]", items)
		}

		if calls != 2 {
			t.Errorf("server was called %d times, want 2", calls)
		}
	})

	t.Run("propagates a fetchPage error", func(t *testing.T) {
		client := newTestGithubClient(t, jsonHandler(t, http.StatusInternalServerError, `{"message":"boom"}`))

		_, err := fetchAllPages[testItem](context.Background(), client, "orgs/foo/dependabot/alerts")
		if err == nil || !strings.Contains(err.Error(), "Failed to fetchPage") {
			t.Errorf("fetchAllPages() error = %v, want it to mention fetchPage", err)
		}
	})
}
