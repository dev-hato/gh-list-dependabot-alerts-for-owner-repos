package main

import (
	"fmt"
	"io"
	"os"

	"github.com/cockroachdb/errors"
)

// fatal logs err with its stack trace to stderr and exits abnormally.
func fatal(err error) {
	if fatalWithExitErr := fatalWithExit(err, os.Stderr, os.Exit); fatalWithExitErr != nil {
		panic(fatalWithExitErr)
	}
}

// fatalWithExit logs err with its stack trace to w (stderr in production) and calls exit(1).
// It is split out from fatal so tests can inject a non-exiting exit func and capture the output.
func fatalWithExit(err error, w io.Writer, exit func(code int)) error {
	if _, fprintfErr := fmt.Fprintf(w, "%+v\n", err); fprintfErr != nil {
		return errors.Join(err, errors.Wrap(fprintfErr, "Failed to fmt.Fprintf"))
	}

	exit(1)
	return nil
}
