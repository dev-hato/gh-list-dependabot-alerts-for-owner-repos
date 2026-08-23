package main

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/v90/github"
)

func TestToSmallDependabotAlert(t *testing.T) {
	t.Parallel()

	createdAt := github.Timestamp{Time: time.Date(2026, 5, 19, 21, 32, 23, 0, time.UTC)}

	tests := map[string]struct {
		alert github.DependabotAlert
		want  SmallDependabotAlert
	}{
		"full alert": {
			alert: github.DependabotAlert{
				Number: new(3),
				State:  new("open"),
				Dependency: &github.Dependency{
					ManifestPath: new("package-lock.json"),
					Scope:        new("runtime"),
				},
				SecurityAdvisory: &github.DependabotSecurityAdvisory{
					Summary:  new("Prototype Pollution in lodash"),
					Severity: new("high"),
				},
				HTMLURL:   new("https://github.com/octocat/Hello-World/security/dependabot/3"),
				CreatedAt: &createdAt,
				// Repository is intentionally set here to confirm toSmallDependabotAlert does NOT copy it;
				// callers are responsible for attaching it themselves.
				Repository: &github.Repository{FullName: new("octocat/Hello-World")},
			},
			want: SmallDependabotAlert{
				Number: new(3),
				State:  new("open"),
				Dependency: &github.Dependency{
					ManifestPath: new("package-lock.json"),
					Scope:        new("runtime"),
				},
				SecurityAdvisory: &SmallDependabotSecurityAdvisory{
					Summary:  new("Prototype Pollution in lodash"),
					Severity: new("high"),
				},
				HTMLURL:   new("https://github.com/octocat/Hello-World/security/dependabot/3"),
				CreatedAt: &createdAt,
				// Repository is intentionally omitted here to confirm toSmallDependabotAlert does NOT copy it.
				Repository: nil,
			},
		},
		"nil security advisory": {
			alert: github.DependabotAlert{
				Number: new(1),
				State:  new("open"),
			},
			want: SmallDependabotAlert{
				Number: new(1),
				State:  new("open"),
			},
		},
		"zero-value alert": {
			alert: github.DependabotAlert{},
			want:  SmallDependabotAlert{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := toSmallDependabotAlert(tt.alert, nil)

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("toSmallDependabotAlert() mismatch (-want +got):\n%s", diff)
			}

			// toSmallDependabotAlert must reuse the Dependency pointer, not copy it.
			if got.Dependency != tt.alert.Dependency {
				t.Errorf("Dependency = %v, want the same pointer as %v", got.Dependency, tt.alert.Dependency)
			}
		})
	}
}
