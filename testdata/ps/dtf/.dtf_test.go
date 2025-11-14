package dtf

import (
	"testing"
)

// Copyright (c) 2025, João Breno. See the license.

type T = testing.T

func TestSub(t *T) {
	a, b := 1, 2
	if s := Sub(a, b); s != a-b {
		t.Errorf("Sub(%d, %d) = %d, want %d", a, b, s, a-b)
	}
}
