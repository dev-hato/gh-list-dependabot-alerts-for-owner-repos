package app

import (
	"context"
	"io"
	"net"
	"net/http"

	"github.com/cli/go-gh/v2/pkg/config"
	"github.com/cockroachdb/errors"
)

// MaxErrorBodyBytes caps how much of a non-2xx response body is read.
// go-gh reads the whole error body inside RequestWithContext, before FetchPage's limits ever see it,
// so the cap has to sit in the transport underneath go-gh.
const MaxErrorBodyBytes = 1 << 20

// ErrErrorBodyTooLarge is what reading a non-2xx response body past MaxErrorBodyBytes fails with.
var ErrErrorBodyTooLarge = errors.Newf("Error response body exceeds %d bytes", MaxErrorBodyBytes)

// LimitedBodyTransport caps every non-2xx response body Base returns at MaxErrorBodyBytes.
type LimitedBodyTransport struct {
	Base http.RoundTripper
}

func (t LimitedBodyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.Base.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to RoundTrip")
	}

	if resp.StatusCode < 200 || 300 <= resp.StatusCode {
		resp.Body = &cappedBody{ReadCloser: resp.Body, remaining: MaxErrorBodyBytes}
	}

	return resp, nil
}

// cappedBody fails with ErrErrorBodyTooLarge once more than remaining bytes have been read.
type cappedBody struct {
	io.ReadCloser
	remaining int64
}

func (b *cappedBody) Read(p []byte) (int, error) {
	// Read one byte past the cap at most, which is enough to tell a body that fits from one that doesn't.
	if b.remaining+1 < int64(len(p)) {
		p = p[:b.remaining+1]
	}

	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)

	if b.remaining < 0 {
		return n, errors.WithStack(ErrErrorBodyTooLarge)
	}

	// io.EOF must reach the caller unwrapped, since readers compare against it with ==.
	if err != nil && !errors.Is(err, io.EOF) {
		return n, errors.Wrap(err, "Failed to Read")
	}

	return n, err
}

// defaultBaseTransport returns the transport go-gh would pick on its own.
// Passing LimitedBodyTransport as ClientOptions.Transport overrides go-gh's choice,
// including the "http_unix_socket" setting, so this repeats that choice underneath it.
func defaultBaseTransport() http.RoundTripper {
	// Like go-gh, fall back to the default transport when the config can't be read or has no socket set.
	cfg, err := config.Read(nil)
	if err != nil {
		return http.DefaultTransport
	}

	socket, err := cfg.Get([]string{"http_unix_socket"})
	if err != nil || socket == "" {
		return http.DefaultTransport
	}

	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer

		conn, err := d.DialContext(ctx, "unix", socket)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to DialContext")
		}

		return conn, nil
	}

	return &http.Transport{DialContext: dial, DialTLSContext: dial, DisableKeepAlives: true}
}
