package app

import "github.com/google/go-github/v90/github"

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

func ToSmallDependabotAlert(alert github.DependabotAlert, repository *SmallRepository) SmallDependabotAlert {
	return SmallDependabotAlert{
		Number:           alert.Number,
		State:            alert.State,
		Dependency:       alert.Dependency,
		SecurityAdvisory: ToSmallSecurityAdvisory(alert),
		HTMLURL:          alert.HTMLURL,
		CreatedAt:        alert.CreatedAt,
		Repository:       repository,
	}
}

// ToSmallSecurityAdvisory trims an alert's security advisory to summary and severity, or nil when it has none.
func ToSmallSecurityAdvisory(alert github.DependabotAlert) *SmallDependabotSecurityAdvisory {
	if alert.SecurityAdvisory == nil {
		return nil
	}

	return &SmallDependabotSecurityAdvisory{
		Summary:  alert.SecurityAdvisory.Summary,
		Severity: alert.SecurityAdvisory.Severity,
	}
}
