package psfitt

import (
	tt "testing"
)

// Copyright (c) 2025, João Breno. See the license.

func FuzzSub(f *tt.F) {
	f.Add(1, 2)
	f.Add(2, 3)
	f.Fuzz(func(t *tt.T, a, b int) {
		if s := Sub(a, b); s != a-b {
			t.Errorf("Sub(%d, %d) = %d, want %d", a, b, s, a-b)
		}
	})
}
