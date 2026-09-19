package app_test

import (
	"testing"
	"time"

	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/app"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/v91/github"
)

func TestDependabotAlertToSmall(t *testing.T) {
	t.Parallel()

	createdAt := github.Timestamp{Time: time.Date(2026, 5, 19, 21, 32, 23, 0, time.UTC)}

	tests := map[string]struct {
		alert app.DependabotAlert
		want  app.SmallDependabotAlert
	}{
		"full alert": {
			alert: app.DependabotAlert{
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
				// Repository is intentionally set here: ToSmall attaches only the repository it's handed,
				// and reading one off the alert is ToSmallRepository's job, not ToSmall's.
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
				// Repository is intentionally omitted here: ToSmall was handed nil, so nothing is attached.
				Repository: nil,
			},
		},
		"nil security advisory": {
			alert: app.DependabotAlert{
				Number: new(1),
				State:  new("open"),
			},
			want: app.SmallDependabotAlert{
				Number: new(1),
				State:  new("open"),
			},
		},
		"zero-value alert": {
			alert: app.DependabotAlert{},
			want:  app.SmallDependabotAlert{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := tt.alert.ToSmall(nil)

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ToSmall() mismatch (-want +got):\n%s", diff)
			}

			// ToSmall must reuse the Dependency pointer, not copy it.
			if got.Dependency != tt.alert.Dependency {
				t.Errorf("Dependency = %v, want the same pointer as %v", got.Dependency, tt.alert.Dependency)
			}
		})
	}
}

func TestDependabotAlertToSmallSecurityAdvisory(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		alert app.DependabotAlert
		want  *app.SmallDependabotSecurityAdvisory
	}{
		"nil security advisory": {
			alert: app.DependabotAlert{},
			want:  nil,
		},
		"summary and severity set": {
			alert: app.DependabotAlert{
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
			alert: app.DependabotAlert{
				SecurityAdvisory: &github.DependabotSecurityAdvisory{},
			},
			want: &app.SmallDependabotSecurityAdvisory{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := tt.alert.ToSmallSecurityAdvisory()

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

func TestDependabotAlertToSmallRepository(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		alert app.DependabotAlert
		want  *app.SmallRepository
	}{
		"nil repository": {
			alert: app.DependabotAlert{},
			want:  nil,
		},
		"repository with full name": {
			alert: app.DependabotAlert{
				Repository: &github.Repository{FullName: new("octocat/Hello-World")},
			},
			want: &app.SmallRepository{FullName: new("octocat/Hello-World")},
		},
		"repository without full name": {
			alert: app.DependabotAlert{
				Repository: &github.Repository{},
			},
			want: &app.SmallRepository{FullName: nil},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := tt.alert.ToSmallRepository()

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ToSmallRepository() mismatch (-want +got):\n%s", diff)
			}

		})
	}
}
