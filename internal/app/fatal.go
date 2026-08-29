package app

import (
	"fmt"
	"io"

	"github.com/cockroachdb/errors"
)

// Fatal logs err with its stack trace to w (stderr in production) and calls exit(1).
// w and exit are taken as parameters so tests can inject a non-exiting exit func and capture the output.
func Fatal(err error, w io.Writer, exit func(code int)) error {
	if _, fprintfErr := fmt.Fprintf(w, "%+v\n", err); fprintfErr != nil {
		return errors.Join(err, errors.Wrap(fprintfErr, "Failed to fmt.Fprintf"))
	}

	exit(1)
	return nil
}
