# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) for this repository.

## Overview

This is a `gh` (GitHub CLI) extension, written in Go.
It fetches Dependabot alerts for every repository owned by a GitHub organization or user.
It prints the results as a JSON array on stdout.
It authenticates via the `gh` CLI's own credentials.
This codebase does no separate token handling.

## Commands

Local run (requires `gh auth login` to already be done). First, install your local code as an extension:

```sh
gh extension install .
```

Then, each time you change the code and want to verify it, run the following command:

```sh
go build && gh list-dependabot-alerts-for-owner-repos <args>
```

## Architecture

The program has two entry paths in `main.go`.
They are chosen by the mutually exclusive `--org`/`--username` flags.
Both paths converge on the same output shape: `[]SmallDependabotAlert`.
It's JSON-marshaled and printed to stdout.

- **Org path** (`list_alerts.go`, `listAlertsForOrg`): calls `orgs/{org}/dependabot/alerts`.
  It returns alerts across all repos in the org directly, including repository info per alert.
- **User path** (`list_alerts.go`, `listAlertsForUser`): no org-wide endpoint exists for user repos.
  So this lists the user's repos (`users/{username}/repos`) and skips archived repos.
  Then it calls `fetchAlertsForRepo` per repository (`repos/{owner}/{repo}/dependabot/alerts`).
  A 403 with "Dependabot alerts are disabled" is treated as "no alerts" (returns `nil, nil`).
  This is because it's an expected state for many repos, not a failure.

Both paths only request `state=open` alerts (`openAlertsURL` in `list_alerts.go`).

**Pagination** (`pagination.go`, `next_path.go`): `fetchAllPages[T]` is a generic helper.
It follows the GitHub API `Link` response header across pages.
It uses `github.RESTClient.RequestWithContext` directly.
It skips the go-github client's built-in pagination.
This lets raw JSON be decoded into whatever type `T` is needed per call site.
`nextPathFromLink` parses the `Link` header with a regex to extract the `rel="next"` URL.
When there's no next link, it returns the current path unchanged.
`fetchAllPages` uses that as its loop-termination signal.

**Rate limiting** (`waiter.go`): a package-level `Waiter` implements exponential backoff.
It uses the AWS-style "Full Jitter" algorithm and increments `attempt` on every call.
It is invoked before every single HTTP request in `fetchPage` (`pagination.go`).
So wait times grow across the entire run, not per-endpoint.
`Call <path>` and `Wait <duration>` progress lines are logged to stderr.
This is expected/normal output, not an error.
It is documented in the readme for users who see the run appear to hang.

**Response shrinking** (`small_dependabot_alert.go`): trims the `github.DependabotAlert` struct.
The result is `SmallDependabotAlert`.
It keeps only the fields the extension's users need.
That's number, state, and dependency.
It also keeps security advisory summary/severity, HTML URL, created-at, and repository full name.
`toSmallDependabotAlert` does the field mapping.
The org and user paths each attach `Repository` slightly differently.
The org endpoint already includes it per-alert.
The user path has to backfill it from the loop variable.

**Error handling** (`fatal.go`): errors are wrapped with `github.com/cockroachdb/errors` throughout.
It uses `errors.Wrap(err, "Failed to ...")` calls to build a stack-trace-annotated chain.
`main` calls `fatal(err)` on any top-level failure.
That prints `%+v` (full chain + stack) to stderr and exits 1.

## Notes

- There are two readmes: `README.md` (English) and `README.ja.md` (Japanese, the original). Keep both in sync with flag/behavior changes.
