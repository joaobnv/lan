package t

import "testing"

// Copyright (c) 2025, João Breno. See the license.

func Test_Sub(t *testing.T) {
	if s := Sub(1, 1); s != 0 {
		t.Errorf("Sub(1, 1) = %d, want 0", s)
	}
}

func Fuzz_Sub(f *testing.F) {
	f.Add(1, 2)
	f.Add(2, 1)
	f.Fuzz(func(t *testing.T, a, b int) {
		if s := Sub(a, b); s != a-b {
			t.Errorf("Sub(%d, %d) = %d, want %d", a, b, s, a-b)
		}
	})
}
