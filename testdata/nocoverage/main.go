// This package contains tests with coverage less than 100%.
package main

// Copyright (c) 2025, João Breno. See the license.

// sum computes the sum of the elements of v.
func sum[T ~int | ~float64](v ...T) T {
	var s T
	for i := range v {
		s += v[i]
	}
	return s
}
