# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is a `gh` (GitHub CLI) extension, written in Go, that fetches Dependabot alerts for every repository owned by a GitHub organization or user and prints them as a JSON array on stdout. It authenticates via the `gh` CLI's own credentials (no separate token handling in this codebase).

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

The program has two entry paths driven by mutually exclusive `--org`/`--username` flags in `main.go`, both converging on the same output shape (`[]SmallDependabotAlert`, JSON-marshaled and printed to stdout):

- **Org path** (`listAlertsForOrg` in `list_alerts.go`): calls the single `orgs/{org}/dependabot/alerts` endpoint, which returns alerts across all repos in the org directly (including repository info per alert).
- **User path** (`listAlertsForUser` in `list_alerts.go`): GitHub has no equivalent org-wide endpoint for user-owned repos, so this lists the user's repos (`users/{username}/repos`), skips archived repos, then calls `fetchAlertsForRepo` per repo (`repos/{owner}/{repo}/dependabot/alerts`). A 403 with "Dependabot alerts are disabled" in the message is treated as "no alerts" (returns `nil, nil`) rather than an error, since that's an expected state for many repos, not a failure.

Both paths only request `state=open` alerts (`openAlertsURL` in `list_alerts.go`).

**Pagination** (`pagination.go`, `next_path.go`): `fetchAllPages[T]` is a generic helper that follows the GitHub API `Link` response header across pages using `github.RESTClient.RequestWithContext` directly (not the go-github client's built-in pagination) so that raw JSON can be decoded into whatever type `T` is needed per call site. `nextPathFromLink` parses the `Link` header with a regex to extract the `rel="next"` URL; when there's no next link, it returns the current path unchanged, which `fetchAllPages` uses as its loop-termination signal.

**Rate limiting** (`waiter.go`): a package-level `Waiter` implements AWS-style "Full Jitter" exponential backoff, incrementing `attempt` on every call. It is invoked before every single HTTP request in `fetchPage` (`pagination.go`), so wait times grow across the entire run, not per-endpoint. `Call <path>` and `Wait <duration>` progress lines are logged to stderr — this is expected/normal output, not an error, and is documented in the README for users who see the run appear to hang.

**Response shrinking** (`small_dependabot_alert.go`): `go-github`'s `github.DependabotAlert` struct is trimmed down to `SmallDependabotAlert`, keeping only the fields the extension's users need (number, state, dependency, security advisory summary/severity, HTML URL, created-at, repository full name). `toSmallDependabotAlert` does the field mapping; the org and user paths each attach `Repository` slightly differently since the org endpoint already includes it per-alert while the user path has to backfill it from the loop variable.

**Error handling** (`fatal.go`): errors are wrapped with `github.com/cockroachdb/errors` throughout (`errors.Wrap(err, "Failed to ...")`) to build a stack-trace-annotated chain, and `main` calls `fatal(err)` on any top-level failure, which prints `%+v` (full chain + stack) to stderr and exits 1.

## Notes

- There are two READMEs: `README.md` (English) and `README.ja.md` (Japanese, the original). Keep both in sync with flag/behavior changes.
