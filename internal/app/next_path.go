package app

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
)

const apiHost = "api.github.com"

// linkRE matches every Link entry whatever its host,
// so a "next" link to another origin is caught by NextPath instead of being skipped as "no next link".
var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)

// LinkHeader is the value of a GitHub API response's Link header.
type LinkHeader string

// NextPath returns the "next" page path found in the header, relative to https://api.github.com/.
// If none is found, it returns path unchanged, signaling "no more pages" to callers that compare the two.
// The header comes from the response, so a next link that would leave api.github.com is an error.
func (l LinkHeader) NextPath(path string) (string, error) {
	for _, m := range linkRE.FindAllStringSubmatch(string(l), -1) {
		if len(m) <= 2 || m[2] != "next" {
			continue
		}

		// url.Parse also rejects control characters, which would otherwise reach the "Call" progress line.
		u, err := url.Parse(m[1])
		if err != nil {
			return "", errors.Wrap(err, "Failed to url.Parse")
		}

		if u.Scheme != "https" || u.Host != apiHost {
			return "", errors.Newf("Next link %q leaves https://%s", m[1], apiHost)
		}

		next := strings.TrimPrefix(u.RequestURI(), "/")

		// go-gh sends a path that parses as an absolute URL as-is, so the relative path must not parse as one.
		ref, err := url.Parse(next)
		if err != nil {
			return "", errors.Wrap(err, "Failed to url.Parse")
		}

		if ref.IsAbs() || ref.Host != "" {
			return "", errors.Newf("Next link %q leaves https://%s", m[1], apiHost)
		}

		return next, nil
	}

	return path, nil
}
