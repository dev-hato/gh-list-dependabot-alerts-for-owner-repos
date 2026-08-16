package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// rewriteTransport redirects every request to target, keeping path and query,
// so a *api.RESTClient built with a fake Host still hits an httptest.Server.
type rewriteTransport struct {
	target *url.URL
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host

	return http.DefaultTransport.RoundTrip(req)
}

func newTestRESTClient(t *testing.T, server *httptest.Server) *api.RESTClient {
	t.Helper()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	client, err := api.NewRESTClient(api.ClientOptions{
		Host:      "github.com",
		AuthToken: "test-token",
		Transport: rewriteTransport{target: target},
	})
	if err != nil {
		t.Fatalf("api.NewRESTClient() error = %v", err)
	}

	return client
}

// resetLimiter clears the package-level rate limiter's accumulated token state.
// It's shared across every test in this package, so without a reset,
// tokens spent by earlier tests would carry over and make later tests wait on the limiter
// for no reason related to what's being tested.
func resetLimiter(t *testing.T) {
	t.Helper()

	limiter = newLimiter()
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("json.Encode() error = %v", err)
	}
}

// registerRepoListHandler registers the "list user's repos" endpoint on mux,
// returning repos as a single page of non-archived repositories.
func registerRepoListHandler(t *testing.T, mux *http.ServeMux, username string, repos []string) {
	t.Helper()

	mux.HandleFunc(fmt.Sprintf("/users/%s/repos", username), func(w http.ResponseWriter, _ *http.Request) {
		list := make([]map[string]any, 0, len(repos))

		for _, name := range repos {
			list = append(list, map[string]any{"name": name, "archived": false})
		}

		writeJSON(t, w, list)
	})
}

// repoAlertsHandler pairs a repository name with the handler serving its dependabot alerts endpoint.
type repoAlertsHandler struct {
	repo    string
	handler http.HandlerFunc
}

// callListAlertsForUser builds an httptest.Server whose mux has the "list user's repos" endpoint registered for repos,
// plus one dependabot-alerts handler per entry in handlers, then calls listAlertsForUser against it.
// The server is closed automatically via t.Cleanup.
func callListAlertsForUser(t *testing.T, username string, repos []string, handlers []repoAlertsHandler) ([]SmallDependabotAlert, error) {
	t.Helper()

	mux := http.NewServeMux()
	registerRepoListHandler(t, mux, username, repos)

	for _, h := range handlers {
		mux.HandleFunc(fmt.Sprintf("/repos/%s/%s/dependabot/alerts", username, h.repo), h.handler)
	}

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := newTestRESTClient(t, server)
	return listAlertsForUser(context.Background(), client, username)
}

// TestListAlertsForUserPreservesRepositoryOrder fetches alerts for repos in parallel,
// with the first repo's response deliberately delayed.
// If the per-repo results were merged as responses arrive (e.g. via a shared slice guarded only by a mutex),
// the faster second repo's alerts would land first.
// Output order must instead always follow repository order, matching the pre-parallelization behavior.
func TestListAlertsForUserPreservesRepositoryOrder(t *testing.T) {
	resetLimiter(t)

	const username = "octocat"

	repos := []string{"slow-repo", "fast-repo"}

	alerts, err := callListAlertsForUser(t, username, repos, []repoAlertsHandler{
		{repos[0], func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(50 * time.Millisecond)
			writeJSON(t, w, []map[string]any{{"number": 1}})
		}},
		{repos[1], func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, []map[string]any{{"number": 2}})
		}},
	})
	if err != nil {
		t.Fatalf("listAlertsForUser() error = %v", err)
	}

	if len(alerts) != 2 {
		t.Fatalf("len(alerts) = %d, want 2", len(alerts))
	}

	if got := *alerts[0].Number; got != 1 {
		t.Errorf("alerts[0].Number = %d, want 1 (slow-repo, first in repository order)", got)
	}

	if got := *alerts[1].Number; got != 2 {
		t.Errorf("alerts[1].Number = %d, want 2 (fast-repo, second in repository order)", got)
	}

	if got := *alerts[0].Repository.FullName; got != username+"/"+repos[0] {
		t.Errorf("alerts[0].Repository.FullName = %s, want %s", got, username+"/"+repos[0])
	}
}

// TestListAlertsForUserFetchesReposConcurrently proves the per-repo fetches are dispatched concurrently,
// not one-at-a-time. Each repo's handler blocks until every repo's request has arrived at the server before responding.
// If the implementation regressed to a sequential loop, repo-a's handler would sit waiting for repo-b/repo-c to arrive,
// but a sequential caller can't start repo-b's request until repo-a's finishes: deadlock,
// caught below as a timeout instead of a hang.
func TestListAlertsForUserFetchesReposConcurrently(t *testing.T) {
	resetLimiter(t)

	const username = "octocat"

	repos := []string{"repo-a", "repo-b", "repo-c"}

	var arrived atomic.Int32

	allArrived := make(chan struct{})

	var closeOnce sync.Once

	handlers := make([]repoAlertsHandler, len(repos))

	for i, name := range repos {
		number := i + 1
		handlers[i] = repoAlertsHandler{name, func(w http.ResponseWriter, _ *http.Request) {
			if arrived.Add(1) == int32(len(repos)) {
				closeOnce.Do(func() { close(allArrived) })
			}

			select {
			case <-allArrived:
			case <-time.After(10 * time.Second):
				t.Errorf("timed out waiting for all %d repo requests to arrive concurrently; only %d arrived (fetches are not overlapping)", len(repos), arrived.Load())
			}

			writeJSON(t, w, []map[string]any{{"number": number}})
		}}
	}

	alerts, err := callListAlertsForUser(t, username, repos, handlers)
	if err != nil {
		t.Fatalf("listAlertsForUser() error = %v", err)
	}

	if len(alerts) != len(repos) {
		t.Fatalf("len(alerts) = %d, want %d", len(alerts), len(repos))
	}
}

// TestListAlertsForUserPropagatesRepoError ensures a failure fetching one repository's alerts
// still surfaces as an error from listAlertsForUser.
func TestListAlertsForUserPropagatesRepoError(t *testing.T) {
	resetLimiter(t)

	const username = "octocat"

	repos := []string{"ok-repo", "broken-repo"}

	_, err := callListAlertsForUser(t, username, repos, []repoAlertsHandler{
		{repos[0], func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, []map[string]any{{"number": 1}})
		}},
		{repos[1], func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(t, w, map[string]any{"message": "boom"})
		}},
	})
	if err == nil {
		t.Fatal("listAlertsForUser() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "fetchAlertsForRepo") {
		t.Errorf("error = %v, want wrapped fetchAlertsForRepo error", err)
	}
}
