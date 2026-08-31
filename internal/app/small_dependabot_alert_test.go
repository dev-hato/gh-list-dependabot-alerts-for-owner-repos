package app_test

import (
	"testing"
	"time"

	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/app"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/v90/github"
)

func TestToSmallDependabotAlert(t *testing.T) {
	t.Parallel()

	createdAt := github.Timestamp{Time: time.Date(2026, 5, 19, 21, 32, 23, 0, time.UTC)}

	tests := map[string]struct {
		alert github.DependabotAlert
		want  app.SmallDependabotAlert
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
			want: app.SmallDependabotAlert{
				Number: new(3),
				State:  new("open"),
				Dependency: &github.Dependency{
					ManifestPath: new("package-lock.json"),
					Scope:        new("runtime"),
				},
				SecurityAdvisory: &app.SmallDependabotSecurityAdvisory{
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
			want: app.SmallDependabotAlert{
				Number: new(1),
				State:  new("open"),
			},
		},
		"zero-value alert": {
			alert: github.DependabotAlert{},
			want:  app.SmallDependabotAlert{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := app.ToSmallDependabotAlert(tt.alert, nil)

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ToSmallDependabotAlert() mismatch (-want +got):\n%s", diff)
			}

			// toSmallDependabotAlert must reuse the Dependency pointer, not copy it.
			if got.Dependency != tt.alert.Dependency {
				t.Errorf("Dependency = %v, want the same pointer as %v", got.Dependency, tt.alert.Dependency)
			}
		})
	}
}

func TestToSmallSecurityAdvisory(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		alert github.DependabotAlert
		want  *app.SmallDependabotSecurityAdvisory
	}{
		"nil security advisory": {
			alert: github.DependabotAlert{},
			want:  nil,
		},
		"summary and severity set": {
			alert: github.DependabotAlert{
				SecurityAdvisory: &github.DependabotSecurityAdvisory{
					Summary:  new("Prototype Pollution in lodash"),
					Severity: new("high"),
				},
			},
			want: &app.SmallDependabotSecurityAdvisory{
				Summary:  new("Prototype Pollution in lodash"),
				Severity: new("high"),
			},
		},
		"advisory present but fields nil": {
			alert: github.DependabotAlert{
				SecurityAdvisory: &github.DependabotSecurityAdvisory{},
			},
			want: &app.SmallDependabotSecurityAdvisory{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := app.ToSmallSecurityAdvisory(tt.alert)

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ToSmallSecurityAdvisory() mismatch (-want +got):\n%s", diff)
			}

			// ToSmallSecurityAdvisory must reuse the field pointers, not copy the strings.
			if tt.alert.SecurityAdvisory != nil {
				if got.Summary != tt.alert.SecurityAdvisory.Summary {
					t.Errorf("Summary = %v, want the same pointer as %v", got.Summary, tt.alert.SecurityAdvisory.Summary)
				}

				if got.Severity != tt.alert.SecurityAdvisory.Severity {
					t.Errorf("Severity = %v, want the same pointer as %v", got.Severity, tt.alert.SecurityAdvisory.Severity)
				}
			}
		})
	}
}
