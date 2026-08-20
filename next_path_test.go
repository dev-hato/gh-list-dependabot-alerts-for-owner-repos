package main

import "testing"

func TestNextPathFromLink(t *testing.T) {
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

			if got := nextPathFromLink(tt.linkHeader, tt.path); got != tt.want {
				t.Errorf("nextPathFromLink(%q, %q) = %q, want %q", tt.linkHeader, tt.path, got, tt.want)
			}
		})
	}
}
