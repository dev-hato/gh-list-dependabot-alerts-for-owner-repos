package slice_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/dev-hato/gh-list-dependabot-alerts-for-owner-repos/internal/slice"
	"github.com/google/go-cmp/cmp"
)

func TestMap(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input []int
		f     func(int) string
		want  []string
	}{
		"applies f to every element, in order": {
			input: []int{1, 2, 3},
			f:     strconv.Itoa,
			want:  []string{"1", "2", "3"},
		},
		"changes the element type": {
			input: []int{1, 22, 333},
			f:     strconv.Itoa,
			want:  []string{"1", "22", "333"},
		},
		"f can be an arbitrary transform": {
			input: []int{1, 2, 3},
			f:     func(n int) string { return strings.Repeat("x", n) },
			want:  []string{"x", "xx", "xxx"},
		},
		"single element": {
			input: []int{42},
			f:     strconv.Itoa,
			want:  []string{"42"},
		},
		"empty input yields an empty, non-nil slice": {
			input: []int{},
			f:     strconv.Itoa,
			want:  []string{},
		},
		"nil input yields an empty, non-nil slice": {
			input: nil,
			f:     strconv.Itoa,
			want:  []string{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			got := slice.Map(tt.input, func(n int) string {
				calls++
				return tt.f(n)
			})

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Map() mismatch (-want +got):\n%s", diff)
			}

			// Map always allocates via make, so the result is non-nil even for empty or nil input.
			if got == nil {
				t.Error("Map() = nil, want a non-nil slice")
			}

			// f must be called exactly once per input element, and never for empty or nil input.
			if calls != len(tt.input) {
				t.Errorf("f was called %d times, want %d", calls, len(tt.input))
			}
		})
	}
}
