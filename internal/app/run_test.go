package app_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/app"
)

// jsonGithubClient adapts jsonHandler to the (*app.GithubClient, error) shape a newClient field needs,
// for the common case of a single-handler test server.
func jsonGithubClient(t *testing.T, status int, body string) (*app.GithubClient, error) {
	t.Helper()

	return newTestGithubClient(t, jsonHandler(t, status, body)), nil
}

func TestRun(t *testing.T) {
	errNoClientForYou := errors.New("no client for you")

	tests := map[string]struct {
		args               []string
		out                io.Writer // nil means capture into a fresh bytes.Buffer
		newClient          func(t *testing.T) (*app.GithubClient, error)
		wantErr            bool
		wantErrContains    string
		wantErrIs          error // optional: also checked with errors.Is
		wantOutputContains string
	}{
		"--org fetches org alerts and prints them as JSON": {
			args: []string{"--org", "foo"},
			newClient: func(t *testing.T) (*app.GithubClient, error) {
				return jsonGithubClient(t, http.StatusOK, `[{"number":1,"state":"open"}]`)
			},
			wantOutputContains: `"number": 1`,
		},
		"no --org fetches the authenticated user's alerts and prints them as JSON": {
			args: nil,
			newClient: func(t *testing.T) (*app.GithubClient, error) {
				mux := http.NewServeMux()
				registerRepoListHandler(t, mux, "alice", []string{"repo"})
				mux.HandleFunc("/repos/alice/repo/dependabot/alerts", jsonHandler(t, http.StatusOK, `[{"number":7,"state":"open"}]`))

				return &app.GithubClient{Rest: newTestRESTClient(t, mux), Limiter: noWaitLimiter()}, nil
			},
			wantOutputContains: `"number": 7`,
		},
		"--help prints usage and exits without error": {
			args: []string{"--help"},
			newClient: func(t *testing.T) (*app.GithubClient, error) {
				t.Fatal("newClient should not be called")
				return nil, nil
			},
			wantOutputContains: "Usage: gh list-dependabot-alerts-for-owner-repos",
		},
		"-h prints usage and exits without error": {
			args: []string{"-h"},
			newClient: func(t *testing.T) (*app.GithubClient, error) {
				t.Fatal("newClient should not be called")
				return nil, nil
			},
			wantOutputContains: "Usage: gh list-dependabot-alerts-for-owner-repos",
		},
		"usage write error is wrapped": {
			args: []string{"--help"},
			out:  failingWriter{},
			newClient: func(t *testing.T) (*app.GithubClient, error) {
				t.Fatal("newClient should not be called")
				return nil, nil
			},
			wantErr:         true,
			wantErrContains: "Failed to io.WriteString",
		},
		"flag parse error is wrapped": {
			args: []string{"--not-a-real-flag"},
			newClient: func(t *testing.T) (*app.GithubClient, error) {
				t.Fatal("newClient should not be called")
				return nil, nil
			},
			wantErr:         true,
			wantErrContains: "Failed to fs.Parse",
		},
		"newClient error is wrapped": {
			args: []string{"--org", "foo"},
			newClient: func(_ *testing.T) (*app.GithubClient, error) {
				return nil, errNoClientForYou
			},
			wantErr:   true,
			wantErrIs: errNoClientForYou,
		},
		"listAlertsForOrg error is wrapped": {
			args: []string{"--org", "foo"},
			newClient: func(t *testing.T) (*app.GithubClient, error) {
				return jsonGithubClient(t, http.StatusInternalServerError, `{"message":"boom"}`)
			},
			wantErr:         true,
			wantErrContains: "Failed to ListAlertsForOrg",
		},
		"listAlertsForUser error is wrapped": {
			args: nil,
			newClient: func(t *testing.T) (*app.GithubClient, error) {
				return jsonGithubClient(t, http.StatusInternalServerError, `{"message":"boom"}`)
			},
			wantErr:         true,
			wantErrContains: "Failed to ListAlertsForUser",
		},
		"output write error is wrapped": {
			args: []string{"--org", "foo"},
			out:  failingWriter{},
			newClient: func(t *testing.T) (*app.GithubClient, error) {
				return jsonGithubClient(t, http.StatusOK, `[]`)
			},
			wantErr:         true,
			wantErrContains: "Failed to fmt.Fprintln",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			out := tt.out

			var buf *bytes.Buffer

			if out == nil {
				buf = &bytes.Buffer{}
				out = buf
			}

			err := app.Run(context.Background(), tt.args, out, func() (*app.GithubClient, error) {
				return tt.newClient(t)
			})

			switch {
			case !tt.wantErr && err != nil:
				t.Fatalf("Run() error = %v, want nil", err)
			case tt.wantErr && err == nil:
				t.Fatal("Run() error = nil, want non-nil")
			case tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains):
				t.Errorf("Run() error = %v, want it to mention %q", err, tt.wantErrContains)
			case tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs):
				t.Errorf("Run() error = %v, want it to wrap %v", err, tt.wantErrIs)
			}

			if tt.wantOutputContains != "" && !strings.Contains(buf.String(), tt.wantOutputContains) {
				t.Errorf("output = %q, want it to contain %q", buf.String(), tt.wantOutputContains)
			}
		})
	}
}
