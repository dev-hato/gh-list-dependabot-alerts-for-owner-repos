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
	var securityAdvisory *SmallDependabotSecurityAdvisory

	if alert.SecurityAdvisory != nil {
		securityAdvisory = &SmallDependabotSecurityAdvisory{
			Summary:  alert.SecurityAdvisory.Summary,
			Severity: alert.SecurityAdvisory.Severity,
		}
	}

	return SmallDependabotAlert{
		Number:           alert.Number,
		State:            alert.State,
		Dependency:       alert.Dependency,
		SecurityAdvisory: securityAdvisory,
		HTMLURL:          alert.HTMLURL,
		CreatedAt:        alert.CreatedAt,
		Repository:       repository,
	}
}
