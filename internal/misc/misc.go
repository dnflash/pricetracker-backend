package misc

import "golang.org/x/exp/constraints"

func Max[T constraints.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func Min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func StringLimit(s string, n int) string {
	if n < 0 {
		return ""
	}
	if n <= 3 {
		return s[:Min(n, len(s))]
	}
	if len(s) > n {
		return s[:n-3] + "..."
	}
	return s
}

func BytesLimit(bs []byte, n int) []byte {
	if n < 0 {
		return nil
	}
	if n <= 3 {
		return bs[:Min(n, len(bs))]
	}
	if len(bs) > n {
		return append(bs[:n-3], "..."...)
	}
	return bs
}
