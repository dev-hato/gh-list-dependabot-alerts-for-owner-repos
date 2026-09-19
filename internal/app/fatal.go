package app

import (
	"fmt"
	"io"

	"github.com/cockroachdb/errors"
)

// FatalReporter reports a top-level failure and terminates the process.
// Out and Exit are fields (stderr and os.Exit in production)
// so tests can capture the output and inject a non-exiting exit func.
type FatalReporter struct {
	Out  io.Writer
	Exit func(code int)
}

// Report logs err with its stack trace to Out and calls Exit(1).
func (f *FatalReporter) Report(err error) error {
	if _, fprintfErr := fmt.Fprintf(f.Out, "%+v\n", err); fprintfErr != nil {
		return errors.Join(err, errors.Wrap(fprintfErr, "Failed to fmt.Fprintf"))
	}

	f.Exit(1)
	return nil
}
