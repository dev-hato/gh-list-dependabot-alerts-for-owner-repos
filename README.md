# gh-list-dependabot-alerts-for-owner-repos

English | [日本語](README.ja.md)

A GitHub CLI extension that lists Dependabot alerts (vulnerability alerts from Dependabot).
It covers every repository owned by an organization or a user.

## What it does

- With `--org`: fetches alerts for every repository under the specified organization.
- With `--username`: fetches alerts for every repository owned by the specified user.

## Requirements

- The `gh` command must be installed and authenticated (e.g. via `gh auth login`).
- You must be authenticated as an account with permission to view Dependabot alerts.

## Installation

```sh
gh extension install dev-hato/gh-list-dependabot-alerts-for-owner-repos
```

## Usage

To fetch alerts for all repositories under an organization:

```sh
gh list-dependabot-alerts-for-owner-repos --org <organization>
```

To fetch alerts for all repositories owned by a user:

```sh
gh list-dependabot-alerts-for-owner-repos --username <username>
```

Specify either `--org` or `--username`. Running the command with both left empty results in an error.

### Options

| Option       | Description              |
| ------------ | ------------------------ |
| `--org`      | Target organization name |
| `--username` | Target username          |

## Example output

The results are printed to standard output as a JSON array like the following:

<!-- jscpd:ignore-start -->

```json
[
  {
    "number": 3,
    "state": "open",
    "dependency": {
      "package": {
        "ecosystem": "npm",
        "name": "lodash"
      },
      "manifest_path": "package-lock.json",
      "scope": "runtime"
    },
    "security_advisory": {
      "summary": "Prototype Pollution in lodash",
      "severity": "high"
    },
    "html_url": "https://github.com/octocat/Hello-World/security/dependabot/3",
    "created_at": "2026-05-19T21:32:23Z",
    "repository": {
      "full_name": "octocat/Hello-World"
    }
  }
]
```

<!-- jscpd:ignore-end -->

## Development

First, install your local code as an extension:

```sh
gh extension install .
```

Then, each time you change the code and want to verify it, run the following command:

```sh
go build && gh list-dependabot-alerts-for-owner-repos <args>
```

## Note: about the wait times

During execution, log lines like `Call <path>` and `Wait <duration>` are printed to standard error.
These appear because a short wait is inserted before each API call as part of the backoff handling.
The more target repositories there are, the longer the run takes to complete.
