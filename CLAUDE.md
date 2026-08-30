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

`main.go` (`package main`) is a thin shell: it just calls `app.Run`.
All logic lives in `internal/app` (`package app`), so tests can use an external `app_test` package.
`go build` at the repository root still produces the `gh` extension binary.
Every tested identifier is exported from `internal/app`.

The program has two entry paths in `run.go` (`app.Run`).
They are chosen by the mutually exclusive `--org`/`--username` flags.
Both paths converge on the same output shape: `[]SmallDependabotAlert`.
It's JSON-marshaled and printed to stdout.

- **Org path** (`list_alerts.go`, `ListAlertsForOrg`): calls `orgs/{org}/dependabot/alerts`.
  It returns alerts across all repos in the org directly, including repository info per alert.
- **User path** (`list_alerts.go`, `ListAlertsForUser`): no org-wide endpoint exists for user repos.
  So this lists the user's repos (`users/{username}/repos`) and skips archived repos.
  Then it calls `FetchAlertsForRepo` per repository (`repos/{owner}/{repo}/dependabot/alerts`).
  These calls run in parallel across repositories via `golang.org/x/sync/errgroup`.
  Results are written into an index-aligned slice, one slot per repository.
  So output order matches repository order, regardless of which fetch finishes first.
  The shared rate limiter (see below) keeps the combined request rate in check across goroutines.
  A 403 with "Dependabot alerts are disabled" is treated as "no alerts" (returns `nil, nil`).
  This is because it's an expected state for many repos, not a failure.

Both paths only request `state=open` alerts (`OpenAlertsURL` in `list_alerts.go`).

**Pagination** (`pagination.go`, `next_path.go`): `FetchAllPages[T]` is a generic helper.
It follows the GitHub API `Link` response header across pages.
It uses `github.RESTClient.RequestWithContext` directly.
It skips the go-github client's built-in pagination.
This lets raw JSON be decoded into whatever type `T` is needed per call site.
`NextPathFromLink` parses the `Link` header with a regular expression to extract the `rel="next"` URL.
When there's no next link, it returns the current path unchanged.
`FetchAllPages` uses that as its loop-termination signal.

**Rate limiting** (`rate_limiter.go`): a package-level `limiter` throttles outgoing requests.
It's a `golang.org/x/time/rate.Limiter`, built via `rate.NewLimiter(5, 1)`.
That's 5 requests per second, with a burst size of one.
`FetchPage` (`pagination.go`) calls `Limiter.Wait(ctx)` before every single HTTP request.
That blocks until the limiter admits the request.
`client` and `limiter` are bundled into one `*GithubClient` (`pagination.go`).
It's defined as `{Rest *api.RESTClient, Limiter *rate.Limiter}`.
Every fetch always needs both together, so they travel as a single value.

That value is threaded through as a parameter.
It comes down from `app.Run`, wired via `newClient` (`app.NewDefaultGithubClient` in production).
So tests can exercise the whole call chain against a fake HTTP server.
That covers `run_test.go`, `list_alerts_test.go`, and `pagination_test.go`.
The test files are `package app_test` (black-box); shared helpers live in `restclient_test.go`.
Tests build one via `newTestGithubClient()` and `noWaitLimiter()` (`restclient_test.go`).
That limiter never blocks.

`rate.Limiter` is safe for concurrent use.
The production `limiter` is one process-wide instance shared across all goroutines.
This matters because `ListAlertsForUser` fetches repositories in parallel.
Keep `limiter` a single shared instance passed down to every goroutine.
A limiter constructed fresh per call or per goroutine would stop throttling entirely.
Each fresh instance starts unthrottled, no matter how many requests other goroutines just made.
`FetchPage` writes its `Call <path>` progress line straight to `os.Stderr`.
This is expected/normal output, not an error.
It is documented in the readme for users who see the run appear to hang.

**Response shrinking** (`small_dependabot_alert.go`): trims the `github.DependabotAlert` struct.
The result is `SmallDependabotAlert`.
It keeps only the fields the extension's users need.
That's number, state, and dependency.
It also keeps security advisory summary/severity, HTML URL, created-at, and repository full name.
`ToSmallDependabotAlert` does the field mapping.
The org and user paths each attach `Repository` slightly differently.
The org endpoint already includes it per-alert.
The user path has to backfill it from the loop variable.

**Error handling** (`fatal.go`): errors are wrapped with `github.com/cockroachdb/errors` throughout.
It uses `errors.Wrap(err, "Failed to ...")` calls to build a stack-trace-annotated chain.
`main` calls `app.Fatal(err)` on any top-level failure.
That prints `%+v` (full chain + stack) to stderr and exits 1.

## Notes

- There are two readmes: `README.md` (English) and `README.ja.md` (Japanese, the original). Keep both in sync with flag/behavior changes.
