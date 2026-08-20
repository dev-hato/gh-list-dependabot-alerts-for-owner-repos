package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// runTestCase is the table shape for TestRun.
// Named so checkRunResult can take it as a parameter,
// keeping the assertion logic out of TestRun's own cyclomatic complexity.
type runTestCase struct {
	args               []string
	out                io.Writer // nil means capture into a fresh bytes.Buffer
	newClient          func(t *testing.T) (*githubClient, error)
	wantErr            bool
	wantErrContains    string
	wantErrIs          error // optional: also checked with errors.Is
	wantOutputContains string
}

func checkRunResult(t *testing.T, tt runTestCase, buf *bytes.Buffer, err error) {
	t.Helper()

	switch {
	case !tt.wantErr && err != nil:
		t.Fatalf("run() error = %v, want nil", err)
	case tt.wantErr && err == nil:
		t.Fatal("run() error = nil, want non-nil")
	case tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains):
		t.Errorf("run() error = %v, want it to mention %q", err, tt.wantErrContains)
	case tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs):
		t.Errorf("run() error = %v, want it to wrap %v", err, tt.wantErrIs)
	}

	if tt.wantOutputContains != "" && !strings.Contains(buf.String(), tt.wantOutputContains) {
		t.Errorf("output = %q, want it to contain %q", buf.String(), tt.wantOutputContains)
	}
}

func TestRun(t *testing.T) {
	errNoClientForYou := errors.New("no client for you")

	tests := map[string]runTestCase{
		"--org fetches org alerts and prints them as JSON": {
			args: []string{"--org", "foo"},
			newClient: func(t *testing.T) (*githubClient, error) {
				client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					mustFprint(t, w, `[{"number":1,"state":"open"}]`)
				}))

				return &githubClient{rest: client, limiter: noWaitLimiter()}, nil
			},
			wantOutputContains: `"number": 1`,
		},
		"--username fetches user alerts and prints them as JSON": {
			args: []string{"--username", "alice"},
			newClient: func(t *testing.T) (*githubClient, error) {
				mux := http.NewServeMux()
				mux.HandleFunc("/users/alice/repos", func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					mustFprint(t, w, `[{"name":"repo","archived":false}]`)
				})
				mux.HandleFunc("/repos/alice/repo/dependabot/alerts", func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					mustFprint(t, w, `[{"number":7,"state":"open"}]`)
				})

				client := newTestRESTClient(t, mux)

				return &githubClient{rest: client, limiter: noWaitLimiter()}, nil
			},
			wantOutputContains: `"number": 7`,
		},
		"neither flag set is an error": {
			args: nil,
			newClient: func(t *testing.T) (*githubClient, error) {
				t.Fatal("newClient should not be called")
				return nil, nil
			},
			wantErr:         true,
			wantErrContains: "org and username are both empty",
		},
		"flag parse error is wrapped": {
			args: []string{"--not-a-real-flag"},
			newClient: func(t *testing.T) (*githubClient, error) {
				t.Fatal("newClient should not be called")
				return nil, nil
			},
			wantErr:         true,
			wantErrContains: "Failed to fs.Parse",
		},
		"newClient error is wrapped": {
			args: []string{"--org", "foo"},
			newClient: func(_ *testing.T) (*githubClient, error) {
				return nil, errNoClientForYou
			},
			wantErr:   true,
			wantErrIs: errNoClientForYou,
		},
		"listAlertsForOrg error is wrapped": {
			args: []string{"--org", "foo"},
			newClient: func(t *testing.T) (*githubClient, error) {
				client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					mustFprint(t, w, `{"message":"boom"}`)
				}))

				return &githubClient{rest: client, limiter: noWaitLimiter()}, nil
			},
			wantErr:         true,
			wantErrContains: "Failed to listAlertsForOrg",
		},
		"listAlertsForUser error is wrapped": {
			args: []string{"--username", "alice"},
			newClient: func(t *testing.T) (*githubClient, error) {
				client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					mustFprint(t, w, `{"message":"boom"}`)
				}))

				return &githubClient{rest: client, limiter: noWaitLimiter()}, nil
			},
			wantErr:         true,
			wantErrContains: "Failed to listAlertsForUser",
		},
		"output write error is wrapped": {
			args: []string{"--org", "foo"},
			out:  failingWriter{},
			newClient: func(t *testing.T) (*githubClient, error) {
				client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					mustFprint(t, w, `[]`)
				}))

				return &githubClient{rest: client, limiter: noWaitLimiter()}, nil
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

			err := run(context.Background(), tt.args, out, func() (*githubClient, error) {
				return tt.newClient(t)
			})
			checkRunResult(t, tt, buf, err)
		})
	}
}
