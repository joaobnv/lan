// This package serve for the test of staticcheck reporting a unused function. This function is mult.
// Note that we dont want that mult be reported two times as unused. We want Lan to handle this case.
// This case occur because mult is unused by the tests and is unused in the non-test code.
package staticcheckunusedfunc

// Copyright (c) 2025, João Breno. See the license.

// Sum computes the sum of the elements of v.
func Sum[T ~int | ~float64](v ...T) T {
	var s T
	for i := range v {
		s += v[i]
	}
	return s
}

// mult computes the multiplication of the elements of v.
func mult[T ~int | ~float64](v ...T) T {
	var s T = 1
	for i := range v {
		s *= v[i]
	}
	return s
}
