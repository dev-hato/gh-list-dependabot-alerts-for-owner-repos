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
	"github.com/google/go-cmp/cmp"
)

// jsonGithubClient adapts jsonHandler to the (*app.GithubClient, error) shape a newClient field needs,
// for the common case of a single-handler test server.
func jsonGithubClient(t *testing.T, status int, body string) (*app.GithubClient, error) {
	t.Helper()

	return newTestGithubClient(t, jsonHandler(t, status, body)), nil
}

// newUserReposClient returns a test client serving the authenticated user's repo list (a single repo "alice/repo") plus
// that repo's dependabot-alerts endpoint returning alertsJSON.
func newUserReposClient(t *testing.T, alertsJSON string) *app.GithubClient {
	t.Helper()

	mux := http.NewServeMux()
	registerRepoListHandler(t, mux, "alice", []string{"repo"})
	mux.HandleFunc("/repos/alice/repo/dependabot/alerts", jsonHandler(t, http.StatusOK, alertsJSON))
	return newTestGithubClient(t, mux)
}

func TestNewFlagSet(t *testing.T) {
	t.Parallel()

	t.Run("registers the org, help and h flags", func(t *testing.T) {
		t.Parallel()

		fs, _ := app.NewFlagSet()

		for _, name := range []string{"org", "help", "h"} {
			if fs.Lookup(name) == nil {
				t.Errorf("NewFlagSet() flag set is missing the %q flag", name)
			}
		}
	})

	t.Run("parses args into the CLIOptions it returns", func(t *testing.T) {
		t.Parallel()

		tests := map[string]struct {
			args []string
			want app.CLIOptions
		}{
			"no args uses defaults": {args: nil, want: app.CLIOptions{}},
			"--org sets Org":        {args: []string{"--org", "foo"}, want: app.CLIOptions{Org: "foo"}},
			"--help sets Help":      {args: []string{"--help"}, want: app.CLIOptions{Help: true}},
			"-h sets Help":          {args: []string{"-h"}, want: app.CLIOptions{Help: true}},
			"--org and -h together": {args: []string{"--org", "foo", "-h"}, want: app.CLIOptions{Org: "foo", Help: true}},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				fs, got := app.NewFlagSet()

				if err := fs.Parse(tt.args); err != nil {
					t.Fatalf("Parse(%q) error = %v", tt.args, err)
				}

				if diff := cmp.Diff(tt.want, *got); diff != "" {
					t.Errorf("NewFlagSet() mismatch (-want +got):\n%s", diff)
				}
			})
		}
	})
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
				return newUserReposClient(t, `[{"number":7,"state":"open"}]`), nil
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

func TestPrintUsage(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		out             io.Writer // nil means capture into a fresh bytes.Buffer
		wantContains    []string
		wantErrContains string
	}{
		"writes the usage text and every registered flag default": {
			wantContains: []string{
				"Usage: gh list-dependabot-alerts-for-owner-repos",
				"A GitHub CLI extension that lists Dependabot alerts",
				"-org",
				"Target organization name",
				"-help",
				"-h",
			},
		},
		"wraps a write error": {
			out:             failingWriter{},
			wantErrContains: "Failed to io.WriteString",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fs, _ := app.NewFlagSet()
			out := tt.out
			var got *bytes.Buffer

			if out == nil {
				got = &bytes.Buffer{}
				out = got
			}

			err := app.PrintUsage(out, fs)

			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("PrintUsage() error = %v, want it to mention %q", err, tt.wantErrContains)
				}

				return
			}

			if err != nil {
				t.Fatalf("PrintUsage() error = %v, want nil", err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(got.String(), want) {
					t.Errorf("PrintUsage() output = %q, want it to contain %q", got.String(), want)
				}
			}
		})
	}
}

func TestListAlerts(t *testing.T) {
	tests := map[string]struct {
		org             string
		newClient       func(t *testing.T) *app.GithubClient
		want            []app.SmallDependabotAlert
		wantErrContains string
	}{
		"empty org fetches the authenticated user's alerts": {
			org: "",
			newClient: func(t *testing.T) *app.GithubClient {
				return newUserReposClient(t, `[{"number":7,"state":"open"}]`)
			},
			want: []app.SmallDependabotAlert{
				{Number: new(7), State: new("open"), Repository: &app.SmallRepository{FullName: new("alice/repo")}},
			},
		},
		"non-empty org fetches that org's alerts": {
			org: "foo",
			newClient: func(t *testing.T) *app.GithubClient {
				return newTestGithubClient(t, jsonHandler(t, http.StatusOK, `[{"number":3,"state":"open","repository":{"full_name":"foo/bar"}}]`))
			},
			want: []app.SmallDependabotAlert{
				{Number: new(3), State: new("open"), Repository: &app.SmallRepository{FullName: new("foo/bar")}},
			},
		},
		"org path error is wrapped": {
			org: "foo",
			newClient: func(t *testing.T) *app.GithubClient {
				return newTestGithubClient(t, jsonHandler(t, http.StatusInternalServerError, `{"message":"boom"}`))
			},
			wantErrContains: "Failed to ListAlertsForOrg",
		},
		"user path error is wrapped": {
			org: "",
			newClient: func(t *testing.T) *app.GithubClient {
				return newTestGithubClient(t, jsonHandler(t, http.StatusInternalServerError, `{"message":"boom"}`))
			},
			wantErrContains: "Failed to ListAlertsForUser",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := app.ListAlerts(context.Background(), tt.newClient(t), tt.org)

			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("ListAlerts() error = %v, want it to mention %q", err, tt.wantErrContains)
				}

				return
			}

			if err != nil {
				t.Fatalf("ListAlerts() error = %v", err)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ListAlerts() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
