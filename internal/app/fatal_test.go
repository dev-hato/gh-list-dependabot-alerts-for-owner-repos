package app_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/app"
)

func TestFatalReporterReport(t *testing.T) {
	t.Run("logs the error and exits with status 1", func(t *testing.T) {
		errorMessage := "boom"
		var buf bytes.Buffer
		var gotCode int

		reporter := &app.FatalReporter{Out: &buf, Exit: func(code int) { gotCode = code }}

		if err := reporter.Report(errors.New(errorMessage)); err != nil {
			t.Error(err)
		}

		if !strings.Contains(buf.String(), errorMessage) {
			t.Errorf("output = %q, want it to contain %q", buf.String(), errorMessage)
		}

		if gotCode != 1 {
			t.Errorf("exit code = %d, want 1", gotCode)
		}
	})
}
