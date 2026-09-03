package foo

// Copyright (c) 2026, João Breno. See the license.

func Mul[T ~int](n ...T) T {
	var r T = 1
	for _, v := range n {
		r *= v
	}
	return r
}