package futo

import "testing"

// Copyright (c) 2025, João Breno. See the license.

func FuzzSum(f *testing.F) {
	f.Add(1, 2)
	f.Add(2, 1)
	f.Fuzz(func(t *testing.T, a, b int) {
		if s := Sum(a, b); s != a+b {
			t.Errorf("Sub(%d, %d) = %d, want %d", a, b, s, a-b)
		}
	})
}
