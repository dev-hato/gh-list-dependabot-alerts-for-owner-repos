package app

import "github.com/google/go-github/v92/github"

type SmallDependabotSecurityAdvisory struct {
	Summary  *string `json:"summary,omitempty"`
	Severity *string `json:"severity,omitempty"`
}

type SmallRepository struct {
	FullName *string `json:"full_name,omitempty"`
}

type SmallDependabotAlert struct {
	Number           *int                             `json:"number,omitempty"`
	State            *string                          `json:"state,omitempty"`
	Dependency       *github.Dependency               `json:"dependency,omitempty"`
	SecurityAdvisory *SmallDependabotSecurityAdvisory `json:"security_advisory,omitempty"`
	HTMLURL          *string                          `json:"html_url,omitempty"`
	CreatedAt        *github.Timestamp                `json:"created_at,omitempty"`
	Repository       *SmallRepository                 `json:"repository,omitempty"`
}

// DependabotAlert is github.DependabotAlert with this extension's shrinking conversions attached as methods.
// It has the same fields and JSON tags, so alert pages decode straight into it.
type DependabotAlert github.DependabotAlert

// ToSmall trims the alert to the fields the extension's users need, attaching repository as-is.
// The repository is a parameter because the org endpoint ships it with the alert
// while the per-repository endpoint leaves it out for the caller to backfill.
func (a DependabotAlert) ToSmall(repository *SmallRepository) SmallDependabotAlert {
	return SmallDependabotAlert{
		Number:           a.Number,
		State:            a.State,
		Dependency:       a.Dependency,
		SecurityAdvisory: a.ToSmallSecurityAdvisory(),
		HTMLURL:          a.HTMLURL,
		CreatedAt:        a.CreatedAt,
		Repository:       repository,
	}
}

// ToSmallSecurityAdvisory trims the alert's security advisory to summary and severity, or nil when it has none.
func (a DependabotAlert) ToSmallSecurityAdvisory() *SmallDependabotSecurityAdvisory {
	if a.SecurityAdvisory == nil {
		return nil
	}

	return &SmallDependabotSecurityAdvisory{
		Summary:  a.SecurityAdvisory.Summary,
		Severity: a.SecurityAdvisory.Severity,
	}
}

// ToSmallRepository pulls the repository full name off an org-endpoint alert, or nil when it carries none.
func (a DependabotAlert) ToSmallRepository() *SmallRepository {
	if a.Repository == nil {
		return nil
	}

	return &SmallRepository{FullName: a.Repository.FullName}
}
