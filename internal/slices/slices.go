package slices

// Map returns a new slice holding f(v) for every v in src, in the same order.
// The result is always non-nil, even when src is empty or nil.
func Map[T any, U any](src []T, f func(t T) U) []U {
	res := make([]U, len(src))

	for i, t := range src {
		res[i] = f(t)
	}

	return res
}
