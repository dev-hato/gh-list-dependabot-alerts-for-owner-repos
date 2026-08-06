package main

import (
	"crypto/rand"
	"math/big"
	"time"

	"github.com/cockroachdb/errors"
)

var waiter Waiter

type Waiter struct {
	attempt int
}

// Wait returns how long to sleep before the next call, using the Full Jitter algorithm.
// https://aws.amazon.com/jp/blogs/architecture/exponential-backoff-and-jitter/
// sleep = random_between(0, min(cap, base * 2 ** attempt))
func (b *Waiter) Wait() (time.Duration, error) {
	const (
		base              = 200 * time.Millisecond
		backoffCapAttempt = 4
		backoffCap        = base << backoffCapAttempt
	)

	defer func() {
		b.attempt++
	}()

	if b.attempt == 0 {
		return time.Duration(0), nil
	}

	maxSleep := backoffCap

	if b.attempt < backoffCapAttempt {
		maxSleep = base << b.attempt
	}

	n, err := rand.Int(rand.Reader, big.NewInt(maxSleep.Nanoseconds()+1))
	if err != nil {
		return 0, errors.Wrap(err, "Failed to rand.Int")
	}

	return time.Duration(n.Int64()), nil
}
