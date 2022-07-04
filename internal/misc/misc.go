package misc

import (
	"golang.org/x/exp/constraints"
	"regexp"
	"strings"
)

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

var NonAlphanumericRegex = regexp.MustCompile(`[^A-Za-z\d ]+`)
var ExtraSpaceRegex = regexp.MustCompile(`  +`)
var HTMLTagRegex = regexp.MustCompile(`<.*?>`)
var NumRegex = regexp.MustCompile(`\d+`)

func CleanString(s string) string {
	res := NonAlphanumericRegex.ReplaceAllLiteralString(s, " ")
	res = ExtraSpaceRegex.ReplaceAllLiteralString(res, " ")
	res = strings.TrimSpace(res)
	return res
}

func IsNum(s string) bool {
	if s == "" {
		return false
	}
	return len(NumRegex.FindString(s)) == len(s)
}
