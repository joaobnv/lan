package staticcheckok

// Copyright (c) 2025, João Breno. See the license.

// Sum computes the sum of the elements of v.
func Sum[T ~int | ~float64](v ...T) T {
	var s T
	for i := range v {
		s += v[i]
	}
	return s
}
