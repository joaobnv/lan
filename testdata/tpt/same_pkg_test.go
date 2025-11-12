package tpt

import "testing"

// Copyright (c) 2025, João Breno. See the license.

func TestSum(t *testing.T) {
	if s := Sum(1, 1); s != 2 {
		t.Errorf("Sum(1, 1) = %d, want 2", s)
	}
}

func FuzzSum(f *testing.F) {
	f.Add(1, 2)
	f.Add(2, 1)
	f.Fuzz(func(t *testing.T, a, b int) {
		if s := Sum(a, b); s != a+b {
			t.Errorf("Sub(%d, %d) = %d, want %d", a, b, s, a+b)
		}
	})
}
