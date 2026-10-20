package app_test

import (
	"testing"

	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/app"
)

func TestLinkHeaderNextPath(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		linkHeader string
		path       string
		want       string
	}{
		"next link present": {
			linkHeader: `<https://api.github.com/orgs/foo/dependabot/alerts?page=2>; rel="next", <https://api.github.com/orgs/foo/dependabot/alerts?page=5>; rel="last"`,
			path:       "orgs/foo/dependabot/alerts?page=1",
			want:       "orgs/foo/dependabot/alerts?page=2",
		},
		"only prev and last, no next": {
			linkHeader: `<https://api.github.com/orgs/foo/dependabot/alerts?page=4>; rel="prev", <https://api.github.com/orgs/foo/dependabot/alerts?page=5>; rel="last"`,
			path:       "orgs/foo/dependabot/alerts?page=5",
			want:       "orgs/foo/dependabot/alerts?page=5",
		},
		"empty link header": {
			linkHeader: "",
			path:       "orgs/foo/dependabot/alerts",
			want:       "orgs/foo/dependabot/alerts",
		},
		"next is not the first relation listed": {
			linkHeader: `<https://api.github.com/orgs/foo/dependabot/alerts?page=1>; rel="first", <https://api.github.com/orgs/foo/dependabot/alerts?page=3>; rel="next"`,
			path:       "orgs/foo/dependabot/alerts?page=2",
			want:       "orgs/foo/dependabot/alerts?page=3",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := app.LinkHeader(tt.linkHeader).NextPath(tt.path)
			if err != nil {
				t.Fatalf("LinkHeader(%q).NextPath(%q) error = %v, want nil", tt.linkHeader, tt.path, err)
			}

			if got != tt.want {
				t.Errorf("LinkHeader(%q).NextPath(%q) = %q, want %q", tt.linkHeader, tt.path, got, tt.want)
			}
		})
	}
}

func TestLinkHeaderNextPathRejectsHostileLinks(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"absolute URL to another host":      `<https://api.github.com/https://example.com/alerts?page=2>; rel="next"`,
		"plain-HTTP URL to another host":    `<https://api.github.com/http://example.com/alerts?page=2>; rel="next"`,
		"control character in the next URL": "<https://api.github.com/orgs/foo/dependabot/alerts?page=2\x1b[2J>; rel=\"next\"",
		"next link on another host":         `<https://example.com/orgs/foo/dependabot/alerts?page=2>; rel="next"`,
		"next link over plain HTTP":         `<http://api.github.com/orgs/foo/dependabot/alerts?page=2>; rel="next"`,
		"next link on another port":         `<https://api.github.com:8443/orgs/foo/dependabot/alerts?page=2>; rel="next"`,
		"relative next link":                `</orgs/foo/dependabot/alerts?page=2>; rel="next"`,
	}

	for name, linkHeader := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got, err := app.LinkHeader(linkHeader).NextPath("orgs/foo/dependabot/alerts"); err == nil {
				t.Errorf("LinkHeader(%q).NextPath() = %q, want an error", linkHeader, got)
			}
		})
	}
}
