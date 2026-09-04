package sne_test

// Copyright (c) 2025, João Breno. See the license.

import (
	"testing"
	"github.com/joaobnv/lan/sne"
)

func FuzzSub(f *testing.F) {
	f.Add(1, 2)
	f.Add(2, 1)
	f.Fuzz(func(t *testing.T, a, b int) {
		if s := sne.Sub(a, b); s != a-b {
			t.Errorf("Sub(%d, %d) = %d, want %d", a, b, s, a-b)
		}
	})
}
