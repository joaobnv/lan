package t

import "regexp"

// Copyright (c) 2025, João Breno. See the license.

func Sub(a, b int) int {
	return a + b
}

func reg() *regexp.Regexp {
	panic("next line is unreachable")
	return regexp.MustCompile("(")
}

func Append(a []int, b int) []int {
	a = append(a)
	return a
}
