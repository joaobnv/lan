// In this scenario the tests are in a diferent package.
package testsinpkg

// Copyright (c) 2025, João Breno. See the license.

func Sum[T ~int | ~float64](v ...T) T {
	var s T
	for i := range v {
		s += v[i]
	}
	return s
}
