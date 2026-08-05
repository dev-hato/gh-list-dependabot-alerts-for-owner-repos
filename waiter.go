package main

import (
	"math/rand"
	"time"
)

var waiter Waiter

type Waiter struct {
	attempt int
}

// Wait returns how long to sleep before the next call, using the Full Jitter algorithm.
// https://aws.amazon.com/jp/blogs/architecture/exponential-backoff-and-jitter/
// sleep = random_between(0, min(cap, base * 2 ** attempt))
func (b *Waiter) Wait() time.Duration {
	const (
		base              = 200 * time.Millisecond
		backoffCapAttempt = 4
		backoffCap        = base << backoffCapAttempt
	)

	defer func() {
		b.attempt++
	}()

	if b.attempt == 0 {
		return time.Duration(0)
	}

	maxSleep := backoffCap

	if b.attempt < backoffCapAttempt {
		maxSleep = base << b.attempt
	}

	return time.Duration(rand.Int63n(maxSleep.Nanoseconds() + 1))
}
