package app_test

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

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cockroachdb/errors"
	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/app"
	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/slice"
	"github.com/google/go-cmp/cmp"
)

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("json.Encode() error = %v", err)
	}
}

// registerRepoListHandler registers the "list the authenticated user's own repos" endpoint on mux,
// returning repos as a single page of non-archived repositories.
// Each entry carries a bare "name" plus a "full_name" of "owner/<repo>";
// ListAlertsForUser reads full_name, so that's the field the assertions below hinge on.
// It also asserts the request asks for type=owner, so a regression back to the unfiltered
// GET /user/repos scope (which would also pull in collaborator/org repos) fails loudly.
func registerRepoListHandler(t *testing.T, mux *http.ServeMux, owner string, repos []string) {
	t.Helper()

	mux.HandleFunc("/user/repos", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("type"); got != "owner" {
			t.Errorf("GET /user/repos type = %q, want %q", got, "owner")
		}

		list := slice.Map(repos, func(name string) map[string]any {
			return map[string]any{"name": name, "full_name": owner + "/" + name, "archived": false}
		})
		writeJSON(t, w, list)
	})
}

// repoAlertsHandler pairs a repository name with the handler serving its dependabot alerts endpoint.
type repoAlertsHandler struct {
	repo    string
	handler http.HandlerFunc
}

// callListAlertsForUser builds an httptest.Server whose mux has the "list user's repos" endpoint registered for repos,
// plus one dependabot-alerts handler per entry in handlers, then calls ListAlertsForUser against it.
// The server is closed automatically via t.Cleanup.
func callListAlertsForUser(t *testing.T, owner string, repos []string, handlers []repoAlertsHandler) ([]app.SmallDependabotAlert, error) {
	t.Helper()

	mux := http.NewServeMux()
	registerRepoListHandler(t, mux, owner, repos)

	for _, h := range handlers {
		mux.HandleFunc(fmt.Sprintf("/repos/%s/%s/dependabot/alerts", owner, h.repo), h.handler)
	}

	client := newTestGithubClient(t, mux)
	return client.ListAlertsForUser(context.Background())
}

// TestListAlertsForUserPreservesRepositoryOrder fetches alerts for repos in parallel,
// with the first repo's response deliberately delayed.
// If the per-repo results were merged as responses arrive (e.g. via a shared slice guarded only by a mutex),
// the faster second repo's alerts would land first.
// Output order must instead always follow repository order, matching the pre-parallelization behavior.
func TestListAlertsForUserPreservesRepositoryOrder(t *testing.T) {
	const owner = "octocat"

	repos := []string{"slow-repo", "fast-repo"}

	got, err := callListAlertsForUser(t, owner, repos, []repoAlertsHandler{
		{repos[0], func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(50 * time.Millisecond)
			writeJSON(t, w, []map[string]any{{"number": 1}})
		}},
		{repos[1], func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, []map[string]any{{"number": 2}})
		}},
	})
	if err != nil {
		t.Fatalf("ListAlertsForUser() error = %v", err)
	}

	// slow-repo's alert must land first despite finishing last, and fast-repo's second despite finishing first.
	want := []app.SmallDependabotAlert{
		{Number: new(1), Repository: &app.SmallRepository{FullName: new(owner + "/" + repos[0])}},
		{Number: new(2), Repository: &app.SmallRepository{FullName: new(owner + "/" + repos[1])}},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListAlertsForUser() mismatch (-want +got):\n%s", diff)
	}
}

// TestListAlertsForUserFetchesReposConcurrently proves the per-repo fetches are dispatched concurrently,
// not one-at-a-time. Each repo's handler blocks until every repo's request has arrived at the server before responding.
// If the implementation regressed to a sequential loop, repo-a's handler would sit waiting for repo-b/repo-c to arrive,
// but a sequential caller can't start repo-b's request until repo-a's finishes: deadlock,
// caught below as a timeout instead of a hang.
func TestListAlertsForUserFetchesReposConcurrently(t *testing.T) {
	const owner = "octocat"

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

	alerts, err := callListAlertsForUser(t, owner, repos, handlers)
	if err != nil {
		t.Fatalf("ListAlertsForUser() error = %v", err)
	}

	if len(alerts) != len(repos) {
		t.Fatalf("len(alerts) = %d, want %d", len(alerts), len(repos))
	}
}

// TestListAlertsForUserPropagatesRepoError ensures a failure fetching one repository's alerts
// still surfaces as an error from ListAlertsForUser.
func TestListAlertsForUserPropagatesRepoError(t *testing.T) {
	const owner = "octocat"

	repos := []string{"ok-repo", "broken-repo"}

	_, err := callListAlertsForUser(t, owner, repos, []repoAlertsHandler{
		{repos[0], func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, []map[string]any{{"number": 1}})
		}},
		{repos[1], func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(t, w, map[string]any{"message": "boom"})
		}},
	})
	if err == nil {
		t.Fatal("ListAlertsForUser() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "FetchAlertsForRepo") {
		t.Errorf("error = %v, want wrapped fetchAlertsForRepo error", err)
	}
}

func TestOpenAlertsURL(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		scope  app.AlertsScope
		target string
		want   string
	}{
		"org path": {
			scope:  app.OrgScope,
			target: "foo",
			want:   "orgs/foo/dependabot/alerts?state=open",
		},
		"repo path": {
			scope:  app.RepoScope,
			target: "foo/bar",
			want:   "repos/foo/bar/dependabot/alerts?state=open",
		},
		"org with a hyphen and an Enterprise Managed User underscore": {
			scope:  app.OrgScope,
			target: "my-org_ent",
			want:   "orgs/my-org_ent/dependabot/alerts?state=open",
		},
		"repo name starting with a dot": {
			scope:  app.RepoScope,
			target: "foo/.github",
			want:   "repos/foo/.github/dependabot/alerts?state=open",
		},
		"repo name with a dot inside": {
			scope:  app.RepoScope,
			target: "foo_ent/bar.js",
			want:   "repos/foo_ent/bar.js/dependabot/alerts?state=open",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.scope.OpenAlertsURL(tt.target)
			if err != nil {
				t.Fatalf("%q.OpenAlertsURL(%q) error = %v, want nil", tt.scope, tt.target, err)
			}

			if got != tt.want {
				t.Errorf("%q.OpenAlertsURL(%q) = %q, want %q", tt.scope, tt.target, got, tt.want)
			}
		})
	}
}

func TestOpenAlertsURLRejectsHostileTargets(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		scope  app.AlertsScope
		target string
	}{
		"org with a slash":          {scope: app.OrgScope, target: "foo/../../user"},
		"org with dot-dot":          {scope: app.OrgScope, target: ".."},
		"org with a query":          {scope: app.OrgScope, target: "foo?state=dismissed"},
		"org with a fragment":       {scope: app.OrgScope, target: "foo#bar"},
		"org with a percent escape": {scope: app.OrgScope, target: "foo%2F..%2Fuser"},
		"org with a control char":   {scope: app.OrgScope, target: "foo\x1b[2J"},
		"empty org":                 {scope: app.OrgScope, target: ""},
		"repo without a slash":      {scope: app.RepoScope, target: "foo"},
		"repo with extra segments":  {scope: app.RepoScope, target: "foo/bar/../../user"},
		"repo named dot-dot":        {scope: app.RepoScope, target: "foo/.."},
		"repo named dot":            {scope: app.RepoScope, target: "foo/."},
		"unknown scope":             {scope: app.AlertsScope("user"), target: "foo"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.scope.OpenAlertsURL(tt.target)
			if err == nil {
				t.Fatalf("%q.OpenAlertsURL(%q) = %q, want an error", tt.scope, tt.target, got)
			}

			// The target is quoted, so control characters in it are escaped rather than printed raw.
			wantErr := fmt.Sprintf("Invalid %s target %q", tt.scope, tt.target)
			if err.Error() != wantErr {
				t.Errorf("%q.OpenAlertsURL(%q) error = %q, want %q", tt.scope, tt.target, err.Error(), wantErr)
			}

			if got != "" {
				t.Errorf("%q.OpenAlertsURL(%q) = %q, want an empty path", tt.scope, tt.target, got)
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

		got, err := client.ListAlertsForOrg(context.Background(), "foo")
		if err != nil {
			t.Fatalf("ListAlertsForOrg() error = %v, want nil", err)
		}

		want := []app.SmallDependabotAlert{
			{Number: new(1), State: new("open"), Repository: &app.SmallRepository{FullName: new("foo/bar")}},
			{Number: new(2), State: new("open")},
		}

		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("ListAlertsForOrg() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("error from fetchAllPages is wrapped", func(t *testing.T) {
		client := newTestGithubClient(t, jsonHandler(t, http.StatusInternalServerError, `{"message":"boom"}`))

		_, err := client.ListAlertsForOrg(context.Background(), "foo")
		if err == nil || !strings.Contains(err.Error(), "Failed to FetchAllPages") {
			t.Errorf("ListAlertsForOrg() error = %v, want it to mention fetchAllPages", err)
		}
	})

	t.Run("an org that would leave the org endpoint is rejected before any request", func(t *testing.T) {
		client := newTestGithubClient(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected request to %q", r.URL.Path)
		}))

		_, err := client.ListAlertsForOrg(context.Background(), "foo/../../user")
		if err == nil || !strings.Contains(err.Error(), "Failed to OpenAlertsURL") {
			t.Errorf("ListAlertsForOrg() error = %v, want it to mention OpenAlertsURL", err)
		}
	})
}

func TestFetchAlertsForRepo(t *testing.T) {
	tests := map[string]struct {
		handler         func(t *testing.T) http.HandlerFunc
		wantErr         bool
		wantErrContains string
		wantAlertsNil   bool
		checkAlerts     func(t *testing.T, got []app.SmallDependabotAlert)
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
			checkAlerts: func(t *testing.T, got []app.SmallDependabotAlert) {
				want := []app.SmallDependabotAlert{
					{Number: new(1), State: new("open"), Repository: &app.SmallRepository{FullName: new("foo/bar")}},
				}

				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("FetchAlertsForRepo() mismatch (-want +got):\n%s", diff)
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
			wantErrContains: "Failed to FetchAllPages",
		},
		"non-HTTP error is wrapped": {
			handler: func(t *testing.T) http.HandlerFunc {
				return jsonHandler(t, http.StatusOK, `not json`)
			},
			wantErr:         true,
			wantErrContains: "Failed to FetchAllPages",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			client := newTestGithubClient(t, tt.handler(t))
			alerts, err := client.FetchAlertsForRepo(context.Background(), "foo/bar")

			switch {
			case !tt.wantErr && err != nil:
				t.Fatalf("FetchAlertsForRepo() error = %v, want nil", err)
			case tt.wantErr && err == nil:
				t.Fatal("FetchAlertsForRepo() error = nil, want non-nil")
			case tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains):
				t.Errorf("FetchAlertsForRepo() error = %v, want it to mention %q", err, tt.wantErrContains)
			}

			if tt.wantAlertsNil && alerts != nil {
				t.Errorf("FetchAlertsForRepo() alerts = %v, want nil", alerts)
			}

			if tt.checkAlerts != nil {
				tt.checkAlerts(t, alerts)
			}
		})
	}
}

func TestIsDependabotAlertsDisabled(t *testing.T) {
	t.Parallel()

	disabled403 := &api.HTTPError{
		StatusCode: http.StatusForbidden,
		Message:    "Dependabot alerts are disabled for this repository.",
	}

	tests := map[string]struct {
		err  error
		want bool
	}{
		"nil error": {
			err:  nil,
			want: false,
		},
		"non-HTTP error": {
			err:  errors.New("boom"),
			want: false,
		},
		"403 with the disabled-alerts message": {
			err:  disabled403,
			want: true,
		},
		"403 wrapped in another error is still detected": {
			err:  fmt.Errorf("Failed to FetchAllPages: %w", disabled403),
			want: true,
		},
		"403 with a different message": {
			err: &api.HTTPError{
				StatusCode: http.StatusForbidden,
				Message:    "You are forbidden.",
			},
			want: false,
		},
		"non-403 status with the disabled-alerts message": {
			err: &api.HTTPError{
				StatusCode: http.StatusNotFound,
				Message:    "Dependabot alerts are disabled for this repository.",
			},
			want: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := app.IsDependabotAlertsDisabled(tt.err); got != tt.want {
				t.Errorf("IsDependabotAlertsDisabled(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestListAlertsForUser(t *testing.T) {
	t.Run("skips archived and nameless repos, aggregates the rest", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/user/repos", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			if _, err := fmt.Fprint(w, `[
				{"name":"active","full_name":"alice/active","archived":false},
				{"name":"old","full_name":"alice/old","archived":true},
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

		got, err := client.ListAlertsForUser(context.Background())
		if err != nil {
			t.Fatalf("ListAlertsForUser() error = %v, want nil", err)
		}

		want := []app.SmallDependabotAlert{
			{Number: new(9), Repository: &app.SmallRepository{FullName: new("alice/active")}},
		}

		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("ListAlertsForUser() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("error listing repos is wrapped", func(t *testing.T) {
		client := newTestGithubClient(t, jsonHandler(t, http.StatusInternalServerError, `{"message":"boom"}`))

		_, err := client.ListAlertsForUser(context.Background())
		if err == nil || !strings.Contains(err.Error(), "Failed to FetchAllPages") {
			t.Errorf("ListAlertsForUser() error = %v, want it to mention fetchAllPages", err)
		}
	})

	t.Run("error fetching a repo's alerts is wrapped", func(t *testing.T) {
		mux := http.NewServeMux()
		registerRepoListHandler(t, mux, "alice", []string{"broken"})
		mux.HandleFunc("/repos/alice/broken/dependabot/alerts", jsonHandler(t, http.StatusInternalServerError, `{"message":"boom"}`))

		client := newTestGithubClient(t, mux)

		_, err := client.ListAlertsForUser(context.Background())
		if err == nil || !strings.Contains(err.Error(), "Failed to FetchAlertsForRepo") {
			t.Errorf("ListAlertsForUser() error = %v, want it to mention fetchAlertsForRepo", err)
		}
	})

	t.Run("a repo full name that would leave the repo endpoint is rejected before any request", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(
			"/user/repos",
			jsonHandler(t, http.StatusOK, `[{"full_name":"alice/x/../../../user","archived":false}]`),
		)
		mux.HandleFunc("/", func(_ http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected request to %q", r.URL.Path)
		})

		client := newTestGithubClient(t, mux)

		_, err := client.ListAlertsForUser(context.Background())
		if err == nil || !strings.Contains(err.Error(), "Failed to OpenAlertsURL") {
			t.Errorf("ListAlertsForUser() error = %v, want it to mention OpenAlertsURL", err)
		}
	})
}
