package app

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
)

const apiHost = "api.github.com"

var linkRE = regexp.MustCompile(`<https://api\.github\.com/([^>]+)>;\s*rel="([^"]+)"`)

// LinkHeader is the value of a GitHub API response's Link header.
type LinkHeader string

// NextPath returns the "next" page path found in the header.
// If none is found, it returns path unchanged, signaling "no more pages" to callers that compare the two.
// The header comes from the response, so a next link that would leave api.github.com is an error.
func (l LinkHeader) NextPath(path string) (string, error) {
	for _, m := range linkRE.FindAllStringSubmatch(string(l), -1) {
		if 2 < len(m) && m[2] == "next" {
			next := m[1]

			// url.Parse also rejects control characters, which would otherwise reach the "Call" progress line.
			u, err := url.Parse("https://" + apiHost + "/" + next)
			if err != nil {
				return "", errors.Wrap(err, "Failed to url.Parse")
			}

			// go-gh sends a path that starts with a scheme to that URL as-is,
			// so "https://api.github.com/https://example.com/" would leave api.github.com.
			if u.Host != apiHost || strings.Contains(next, "://") {
				return "", errors.Newf("Next link %q leaves %s", next, apiHost)
			}

			return next, nil
		}
	}

	return path, nil
}
