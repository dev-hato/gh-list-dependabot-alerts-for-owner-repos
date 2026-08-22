package main

import (
	"golang.org/x/time/rate"
)

// limiter throttles outgoing GitHub API requests to a steady rate,
// so that per-repository fetches issued in parallel don't burst against GitHub all at once.
var limiter = newLimiter()

func newLimiter() *rate.Limiter {
	return rate.NewLimiter(5, 1)
}
