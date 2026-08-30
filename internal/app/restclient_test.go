package app_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/app"
	"golang.org/x/time/rate"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// noWaitLimiter returns a *rate.Limiter that never blocks, so tests don't pay for real rate limiting.
func noWaitLimiter() *rate.Limiter {
	return rate.NewLimiter(rate.Inf, 0)
}

// newTestGithubClient returns a *app.GithubClient whose requests are redirected to an httptest.Server running handler,
// paired with a Waiter that never blocks.
func newTestGithubClient(t *testing.T, handler http.Handler) *app.GithubClient {
	t.Helper()
	return &app.GithubClient{Rest: newTestRESTClient(t, handler), Limiter: noWaitLimiter()}
}

// jsonHandler returns an http.HandlerFunc that responds with status and body as an application/json response.
// It's the shared shape behind most test HTTP handlers in this package:
// set Content-Type, write the status, write the body.
func jsonHandler(t *testing.T, status int, body string) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)

		if _, err := fmt.Fprint(w, body); err != nil {
			t.Error(err)
		}
	}
}

func newRESTClientWithTransport(t *testing.T, transport http.RoundTripper) *api.RESTClient {
	t.Helper()

	client, err := api.NewRESTClient(api.ClientOptions{
		Host:      "github.com",
		AuthToken: "test-token",
		Transport: transport,
	})
	if err != nil {
		t.Fatal(err)
	}

	return client
}

// newTestRESTClient returns an *api.RESTClient whose requests are redirected to an httptest.Server running handler,
// regardless of the host in the request URL.
func newTestRESTClient(t *testing.T, handler http.Handler) *api.RESTClient {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	return newRESTClientWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = serverURL.Scheme
		req.URL.Host = serverURL.Host
		return http.DefaultTransport.RoundTrip(req)
	}))
}
