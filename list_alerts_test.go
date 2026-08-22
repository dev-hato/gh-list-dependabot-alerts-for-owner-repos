package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

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
		list := make([]map[string]any, len(repos))

		for i, name := range repos {
			list[i] = map[string]any{"name": name, "archived": false}
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

	client := newTestGithubClient(t, mux)
	return listAlertsForUser(context.Background(), client, username)
}

// TestListAlertsForUserPreservesRepositoryOrder fetches alerts for repos in parallel,
// with the first repo's response deliberately delayed.
// If the per-repo results were merged as responses arrive (e.g. via a shared slice guarded only by a mutex),
// the faster second repo's alerts would land first.
// Output order must instead always follow repository order, matching the pre-parallelization behavior.
func TestListAlertsForUserPreservesRepositoryOrder(t *testing.T) {
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

	// slow-repo's alert must land first despite finishing last, and fast-repo's second despite finishing first.
	want := []SmallDependabotAlert{
		{Number: new(1), Repository: &SmallRepository{FullName: new(username + "/" + repos[0])}},
		{Number: new(2), Repository: &SmallRepository{FullName: new(username + "/" + repos[1])}},
	}

	if diff := cmp.Diff(want, alerts); diff != "" {
		t.Errorf("listAlertsForUser() mismatch (-want +got):\n%s", diff)
	}
}

// TestListAlertsForUserFetchesReposConcurrently proves the per-repo fetches are dispatched concurrently,
// not one-at-a-time. Each repo's handler blocks until every repo's request has arrived at the server before responding.
// If the implementation regressed to a sequential loop, repo-a's handler would sit waiting for repo-b/repo-c to arrive,
// but a sequential caller can't start repo-b's request until repo-a's finishes: deadlock,
// caught below as a timeout instead of a hang.
func TestListAlertsForUserFetchesReposConcurrently(t *testing.T) {
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

func TestOpenAlertsURL(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		path string
		want string
	}{
		"org path": {
			path: "orgs/foo/dependabot/alerts",
			want: "orgs/foo/dependabot/alerts?state=open",
		},
		"repo path": {
			path: "repos/foo/bar/dependabot/alerts",
			want: "repos/foo/bar/dependabot/alerts?state=open",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := openAlertsURL(tt.path); got != tt.want {
				t.Errorf("openAlertsURL(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestListAlertsForOrg(t *testing.T) {
	t.Run("success attaches the alert's own repository", func(t *testing.T) {
		client := newTestGithubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/orgs/foo/dependabot/alerts" {
				t.Errorf("path = %q, want /orgs/foo/dependabot/alerts", r.URL.Path)
			}

			w.Header().Set("Content-Type", "application/json")

			if _, err := fmt.Fprint(w, `[
				{"number":1,"state":"open","repository":{"full_name":"foo/bar"}},
				{"number":2,"state":"open"}
			]`); err != nil {
				t.Error(err)
			}
		}))

		alerts, err := listAlertsForOrg(context.Background(), client, "foo")
		if err != nil {
			t.Fatalf("listAlertsForOrg() error = %v, want nil", err)
		}

		want := []SmallDependabotAlert{
			{Number: new(1), State: new("open"), Repository: &SmallRepository{FullName: new("foo/bar")}},
			{Number: new(2), State: new("open")},
		}

		if diff := cmp.Diff(want, alerts); diff != "" {
			t.Errorf("listAlertsForOrg() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("error from fetchAllPages is wrapped", func(t *testing.T) {
		client := newTestGithubClient(t, jsonHandler(t, http.StatusInternalServerError, `{"message":"boom"}`))

		_, err := listAlertsForOrg(context.Background(), client, "foo")
		if err == nil || !strings.Contains(err.Error(), "Failed to fetchAllPages") {
			t.Errorf("listAlertsForOrg() error = %v, want it to mention fetchAllPages", err)
		}
	})
}

func TestFetchAlertsForRepo(t *testing.T) {
	tests := map[string]struct {
		handler         func(t *testing.T) http.HandlerFunc
		wantErr         bool
		wantErrContains string
		wantAlertsNil   bool
		checkAlerts     func(t *testing.T, alerts []SmallDependabotAlert)
	}{
		"success backfills the repository from ownerRepo": {
			handler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/repos/foo/bar/dependabot/alerts" {
						t.Errorf("path = %q, want /repos/foo/bar/dependabot/alerts", r.URL.Path)
					}

					w.Header().Set("Content-Type", "application/json")

					if _, err := fmt.Fprint(w, `[{"number":1,"state":"open"}]`); err != nil {
						t.Error(err)
					}
				}
			},
			checkAlerts: func(t *testing.T, alerts []SmallDependabotAlert) {
				want := []SmallDependabotAlert{
					{Number: new(1), State: new("open"), Repository: &SmallRepository{FullName: new("foo/bar")}},
				}

				if diff := cmp.Diff(want, alerts); diff != "" {
					t.Errorf("fetchAlertsForRepo() mismatch (-want +got):\n%s", diff)
				}
			},
		},
		"403 disabled-alerts message returns nil, nil": {
			handler: func(t *testing.T) http.HandlerFunc {
				return jsonHandler(t, http.StatusForbidden, `{"message":"Dependabot alerts are disabled for this repository."}`)
			},
			wantAlertsNil: true,
		},
		"403 with a different message is an error": {
			handler: func(t *testing.T) http.HandlerFunc {
				return jsonHandler(t, http.StatusForbidden, `{"message":"You are forbidden."}`)
			},
			wantErr:       true,
			wantAlertsNil: true,
		},
		"404 is an error, not treated as disabled alerts": {
			handler: func(t *testing.T) http.HandlerFunc {
				return jsonHandler(t, http.StatusNotFound, `{"message":"Not Found"}`)
			},
			wantErr:         true,
			wantErrContains: "Failed to fetchAllPages",
		},
		"non-HTTP error is wrapped": {
			handler: func(t *testing.T) http.HandlerFunc {
				return jsonHandler(t, http.StatusOK, `not json`)
			},
			wantErr:         true,
			wantErrContains: "Failed to fetchAllPages",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			client := newTestGithubClient(t, tt.handler(t))
			alerts, err := fetchAlertsForRepo(context.Background(), client, "foo/bar")

			switch {
			case !tt.wantErr && err != nil:
				t.Fatalf("fetchAlertsForRepo() error = %v, want nil", err)
			case tt.wantErr && err == nil:
				t.Fatal("fetchAlertsForRepo() error = nil, want non-nil")
			case tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains):
				t.Errorf("fetchAlertsForRepo() error = %v, want it to mention %q", err, tt.wantErrContains)
			}

			if tt.wantAlertsNil && alerts != nil {
				t.Errorf("fetchAlertsForRepo() alerts = %v, want nil", alerts)
			}

			if tt.checkAlerts != nil {
				tt.checkAlerts(t, alerts)
			}
		})
	}
}

func TestListAlertsForUser(t *testing.T) {
	t.Run("skips archived and nameless repos, aggregates the rest", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/users/alice/repos", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			if _, err := fmt.Fprint(w, `[
				{"name":"active","archived":false},
				{"name":"old","archived":true},
				{"archived":false}
			]`); err != nil {
				t.Error(err)
			}
		})
		mux.HandleFunc("/repos/alice/active/dependabot/alerts", jsonHandler(t, http.StatusOK, `[{"number":9}]`))
		mux.HandleFunc("/repos/alice/old/dependabot/alerts", func(_ http.ResponseWriter, _ *http.Request) {
			t.Error("an archived repository should not be queried for alerts")
		})

		client := newTestGithubClient(t, mux)

		alerts, err := listAlertsForUser(context.Background(), client, "alice")
		if err != nil {
			t.Fatalf("listAlertsForUser() error = %v, want nil", err)
		}

		want := []SmallDependabotAlert{
			{Number: new(9), Repository: &SmallRepository{FullName: new("alice/active")}},
		}

		if diff := cmp.Diff(want, alerts); diff != "" {
			t.Errorf("listAlertsForUser() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("error listing repos is wrapped", func(t *testing.T) {
		client := newTestGithubClient(t, jsonHandler(t, http.StatusInternalServerError, `{"message":"boom"}`))

		_, err := listAlertsForUser(context.Background(), client, "alice")
		if err == nil || !strings.Contains(err.Error(), "Failed to fetchAllPages") {
			t.Errorf("listAlertsForUser() error = %v, want it to mention fetchAllPages", err)
		}
	})

	t.Run("error fetching a repo's alerts is wrapped", func(t *testing.T) {
		mux := http.NewServeMux()
		registerRepoListHandler(t, mux, "alice", []string{"broken"})
		mux.HandleFunc("/repos/alice/broken/dependabot/alerts", jsonHandler(t, http.StatusInternalServerError, `{"message":"boom"}`))

		client := newTestGithubClient(t, mux)

		_, err := listAlertsForUser(context.Background(), client, "alice")
		if err == nil || !strings.Contains(err.Error(), "Failed to fetchAlertsForRepo") {
			t.Errorf("listAlertsForUser() error = %v, want it to mention fetchAlertsForRepo", err)
		}
	})
}
