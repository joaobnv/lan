package f_test

import (
	pf "ps/f"
	"testing"
)

// Copyright (c) 2025, João Breno. See the license.

func FuzzSub(f *testing.F) {
	f.Add(1, 2)
	f.Add(2, 3)
	f.Fuzz(func(t *testing.T, a, b int) {
		if s := pf.Sub(a, b); 0 != a-b {
			t.Errorf("Sub(%s, %d) = %d, want %d", a, b, s, -1)
		}
	})
}
