package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	errNoClientForYou := errors.New("no client for you")

	tests := map[string]struct {
		args               []string
		out                io.Writer // nil means capture into a fresh bytes.Buffer
		newClient          func(t *testing.T) (*githubClient, error)
		wantErr            bool
		wantErrContains    string
		wantErrIs          error // optional: also checked with errors.Is
		wantOutputContains string
	}{
		"--org fetches org alerts and prints them as JSON": {
			args: []string{"--org", "foo"},
			newClient: func(t *testing.T) (*githubClient, error) {
				client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")

					if _, err := fmt.Fprint(w, `[{"number":1,"state":"open"}]`); err != nil {
						t.Error(err)
					}
				}))

				return &githubClient{rest: client, limiter: noWaitLimiter()}, nil
			},
			wantOutputContains: `"number": 1`,
		},
		"--username fetches user alerts and prints them as JSON": {
			args: []string{"--username", "alice"},
			newClient: func(t *testing.T) (*githubClient, error) {
				mux := http.NewServeMux()
				mux.HandleFunc("/users/alice/repos", func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")

					if _, err := fmt.Fprint(w, `[{"name":"repo","archived":false}]`); err != nil {
						t.Error(err)
					}
				})
				mux.HandleFunc("/repos/alice/repo/dependabot/alerts", func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")

					if _, err := fmt.Fprint(w, `[{"number":7,"state":"open"}]`); err != nil {
						t.Error(err)
					}
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
			newClient: func(t *testing.T) (*githubClient, error) {
				return nil, errNoClientForYou
			},
			wantErr:   true,
			wantErrIs: errNoClientForYou,
		},
		"listAlertsForOrg error is wrapped": {
			args: []string{"--org", "foo"},
			newClient: func(t *testing.T) (*githubClient, error) {
				client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)

					if _, err := fmt.Fprint(w, `{"message":"boom"}`); err != nil {
						t.Error(err)
					}
				}))

				return &githubClient{rest: client, limiter: noWaitLimiter()}, nil
			},
			wantErr:         true,
			wantErrContains: "Failed to listAlertsForOrg",
		},
		"listAlertsForUser error is wrapped": {
			args: []string{"--username", "alice"},
			newClient: func(t *testing.T) (*githubClient, error) {
				client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)

					if _, err := fmt.Fprint(w, `{"message":"boom"}`); err != nil {
						t.Error(err)
					}
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
				client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")

					if _, err := fmt.Fprint(w, `[]`); err != nil {
						t.Error(err)
					}
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
		})
	}
}
