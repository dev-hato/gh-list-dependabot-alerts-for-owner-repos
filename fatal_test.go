package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestFatalWithExit(t *testing.T) {
	t.Run("logs the error and exits with status 1", func(t *testing.T) {
		var buf bytes.Buffer
		var gotCode int

		if err := fatalWithExit(errors.New("boom"), &buf, func(code int) { gotCode = code }); err != nil {
			t.Error(err)
		}

		if !strings.Contains(buf.String(), "boom") {
			t.Errorf("output = %q, want it to contain %q", buf.String(), "boom")
		}

		if gotCode != 1 {
			t.Errorf("exit code = %d, want 1", gotCode)
		}
	})
}
