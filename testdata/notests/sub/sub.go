package sub

// sub computes the subtraction of the elements of v.
func sub[T ~int | ~float64](v ...T) T {
	var s T
	for i := range v {
		s -= v[i]
	}
	return s
}
