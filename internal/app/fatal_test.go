package app_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/app"
)

func TestFatal(t *testing.T) {
	t.Run("logs the error and exits with status 1", func(t *testing.T) {
		errorMessage := "boom"
		var buf bytes.Buffer
		var gotCode int

		if err := app.Fatal(errors.New(errorMessage), &buf, func(code int) { gotCode = code }); err != nil {
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
