package main

import (
	"fmt"
	"os"

	"github.com/cockroachdb/errors"
)

// fatal logs err with its stack trace to stderr and exits abnormally.
func fatal(err error) {
	if _, fprintfErr := fmt.Fprintf(os.Stderr, "%+v\n", err); fprintfErr != nil {
		panic(errors.Join(err, errors.Wrap(fprintfErr, "Failed to fmt.Fprintf")))
	}

	os.Exit(1)
}
