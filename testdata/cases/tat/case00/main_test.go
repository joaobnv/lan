package main

// Copyright (c) 2026, João Breno. See the license.

type Number interface {
	~int | ~float64 | ~complex128
}

type Multiplier struct{}

func (a Multiplier) Mul[T Number](n1, n2 T, rs ...T) T {
	result := n1 * n2
	for _, n := range rs {
		result *= n
	}
	return result
}
