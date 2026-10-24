package app_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/app"
)

// countingReader counts how many bytes have been read through it.
type countingReader struct {
	r    io.Reader
	read int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += n
	return n, err
}

// jsonResponse returns a round-trip function that answers every request with status and body as JSON.
func jsonResponse(status int, body io.Reader) roundTripFunc {
	return func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(body),
			Request:    req,
		}, nil
	}
}

func TestFetchPageCapsErrorResponseBody(t *testing.T) {
	// A JSON error body several times over the cap, which go-gh would otherwise read in full.
	body := &countingReader{r: io.MultiReader(
		strings.NewReader(`{"message":"`),
		bytes.NewReader(bytes.Repeat([]byte("a"), 4*app.MaxErrorBodyBytes)),
		strings.NewReader(`"}`),
	)}

	client := &app.GithubClient{
		Rest:    newRESTClientWithTransport(t, jsonResponse(http.StatusInternalServerError, body)),
		Limiter: noWaitLimiter(),
		Budget:  fullBudget(),
	}

	_, err := app.NewPageFetcher[testItem](client).FetchPage(t.Context(), "orgs/foo/dependabot/alerts")
	if err == nil || !strings.Contains(err.Error(), app.ErrErrorBodyTooLarge.Error()) {
		t.Errorf("FetchPage() error = %v, want it to mention %q", err, app.ErrErrorBodyTooLarge)
	}

	if app.MaxErrorBodyBytes+1 < body.read {
		t.Errorf("read %d bytes of the error body, want at most %d", body.read, app.MaxErrorBodyBytes+1)
	}
}

func TestLimitedBodyTransport(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		status  int
		body    string
		wantErr error
	}{
		"error body within the cap is read in full": {
			status: http.StatusNotFound,
			body:   strings.Repeat("a", app.MaxErrorBodyBytes),
		},
		"error body over the cap fails": {
			status:  http.StatusNotFound,
			body:    strings.Repeat("a", app.MaxErrorBodyBytes+1),
			wantErr: app.ErrErrorBodyTooLarge,
		},
		"success body is left to FetchPage's own limits": {
			status: http.StatusOK,
			body:   strings.Repeat("a", app.MaxErrorBodyBytes+1),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			transport := app.LimitedBodyTransport{Base: jsonResponse(tt.status, strings.NewReader(tt.body))}

			req, err := http.NewRequestWithContext(
				t.Context(),
				http.MethodGet,
				"https://api.github.com/",
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}

			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip() error = %v, want nil", err)
			}

			defer func() {
				if closeErr := resp.Body.Close(); closeErr != nil {
					t.Error(closeErr)
				}
			}()

			got, err := io.ReadAll(resp.Body)

			switch {
			case tt.wantErr != nil && !errors.Is(err, tt.wantErr):
				t.Errorf("ReadAll() error = %v, want %v", err, tt.wantErr)
			case tt.wantErr == nil && err != nil:
				t.Errorf("ReadAll() error = %v, want nil", err)
			case tt.wantErr == nil && string(got) != tt.body:
				t.Errorf("ReadAll() read %d bytes, want all %d", len(got), len(tt.body))
			}
		})
	}
}
