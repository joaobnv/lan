package sne_test

import (
	"sne"
	"testing"
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
