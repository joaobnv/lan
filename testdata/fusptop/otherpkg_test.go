package fusptop_test

// Copyright (c) 2025, João Breno. See the license.

import (
	"fusptop"
	"testing"
)

func TestSub(t *testing.T) {
	if s := fusptop.Sub(1, 1); s != 0 {
		t.Errorf("Sub(1, 1) = %d, want 0", s)
	}
}
